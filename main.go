// 灵感（Inspiration）——本地合同生成桌面应用。
//
// 入口放在仓库根目录，是因为 Wails 前端资源通过 //go:embed all:frontend/dist 嵌入，
// 而 embed 指令只能引用当前包目录及其子目录。
package main

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"

	"github.com/sheep-ah/inspiration/internal/app"
	"github.com/sheep-ah/inspiration/internal/config"
	"github.com/sheep-ah/inspiration/internal/contract"
	"github.com/sheep-ah/inspiration/internal/converter"
	"github.com/sheep-ah/inspiration/internal/docx"
	"github.com/sheep-ah/inspiration/internal/logging"
	"github.com/sheep-ah/inspiration/internal/store"
	"github.com/sheep-ah/inspiration/internal/template"
	"github.com/sheep-ah/inspiration/internal/version"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	showVersion := flag.Bool("version", false, "打印版本信息并退出")
	inspect := flag.String("inspect", "", "扫描 DOCX 模板并输出占位符与校验结果（JSON），不需要数据目录")
	checkConverters := flag.Bool("check-converters", false, "列出本机可用的 .doc 转换通道（Word / WPS / LibreOffice）")
	showFields := flag.Bool("fields", false, "打印当前版本支持的占位符字段列表（用于确认应用版本）")
	runConverterCheck := flag.Bool("run-converter-check", false, "实际执行一次 .docx→.doc 转换来验证转换通道（需要数据目录）")
	selfCheck := flag.Bool("selfcheck", false, "初始化数据目录与数据库后退出（用于 CI 冒烟测试）")
	dataDir := flag.String("data-dir", "", "覆盖应用数据目录（默认使用系统用户级目录）")
	flag.Parse()

	if *showVersion {
		fmt.Println(version.String())
		return
	}

	if *showFields {
		printFields()
		return
	}

	// 转换器体检：不需要数据目录
	if *checkConverters {
		printConverters()
		return
	}

	// 模板体检：用于批量转换/导入前的检查，不依赖数据目录与数据库
	if strings.TrimSpace(*inspect) != "" {
		if err := inspectTemplate(*inspect); err != nil {
			fmt.Fprintln(os.Stderr, "检查失败:", err)
			os.Exit(2)
		}
		return
	}

	if err := run(*selfCheck || *runConverterCheck, *runConverterCheck, *dataDir); err != nil {
		fmt.Fprintln(os.Stderr, "启动失败:", err)
		// GUI 应用没有控制台：启动失败必须弹窗，否则用户只会看到"点了没反应"
		if !*selfCheck && !*runConverterCheck {
			app.ShowStartupError("灵感启动失败", err.Error()+app.StartupErrorHint(err))
		}
		os.Exit(1)
	}
}

func run(selfCheckOnly bool, verifyConverter bool, dataDirOverride string) error {
	cfg, err := resolveConfig(dataDirOverride)
	if err != nil {
		return err
	}
	log, closer, err := logging.New(cfg.LogsDir, version.IsDev())
	if err != nil {
		return fmt.Errorf("初始化日志失败: %w", err)
	}
	defer func() { _ = closer.Close() }()

	st, err := store.Open(cfg)
	if err != nil {
		return err
	}

	fields := contract.FieldNames()
	conv := converter.New(filepath.Join(cfg.Root, "convert"))
	appInstance := app.New(log, cfg, st, conv,
		template.NewService(st, fields, conv),
		contract.NewService(st, conv),
	)

	if selfCheckOnly {
		if err := st.Close(); err != nil {
			return err
		}
		status := conv.Detect()
		converterInfo := "不可用（无法生成 .doc）"
		if status.Available {
			converterInfo = status.Detail
		}
		fmt.Printf("自检通过：version=%s dataDir=%s\n  转换通道：%s\n", version.String(), cfg.Root, converterInfo)
		if verifyConverter {
			if err := verifyConverterChain(cfg); err != nil {
				return err
			}
		}
		return nil
	}

	err = wails.Run(&options.App{
		Title:     "灵感",
		Width:     1180,
		Height:    800,
		MinWidth:  960,
		MinHeight: 640,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 246, G: 247, B: 249, A: 1},
		OnStartup:        appInstance.Startup,
		OnShutdown:       appInstance.Shutdown,
		Bind: []interface{}{
			appInstance,
		},
		Mac: &mac.Options{
			TitleBar:             mac.TitleBarDefault(),
			WebviewIsTransparent: false,
			WindowIsTranslucent:  false,
			About: &mac.AboutInfo{
				Title:   "灵感",
				Message: fmt.Sprintf("本地合同生成工具\n版本 %s\n\n合同内容与模板仅在本机处理，不会上传。", version.String()),
			},
		},
	})
	if err != nil {
		_ = st.Close()
		return err
	}
	return nil
}

// printFields 打印当前版本支持的占位符字段（排查"模板占位符不被支持"时先看这里）。
func printFields() {
	fmt.Printf("版本：%s\n", version.String())
	fmt.Printf("支持的占位符字段（共 %d 个）：\n", len(contract.Fields))
	for i, f := range contract.Fields {
		fmt.Printf("  %2d. {{%s}}\n", i+1, f.Key)
	}
}

// printConverters 打印各转换通道的可用性（用于排查"为什么不能生成 .doc"）。
func printConverters() {
	conv := converter.New(filepath.Join(os.TempDir(), "inspiration-converter-check"))
	status := conv.Detect()
	fmt.Println("转换通道（按优先级）：")
	for _, ch := range status.Channels {
		mark := "✗"
		if ch.Available {
			mark = "✔"
		}
		fmt.Printf("  %s %-16s %s\n", mark, ch.Name, ch.Detail)
	}
	if status.Available {
		fmt.Printf("\n将使用：%s\n", status.Detail)
	} else {
		fmt.Println("\n没有可用通道：生成 .doc 会失败（建议安装 Microsoft Word、WPS Office 或 LibreOffice）")
	}
}

// verifyConverterChain 用真实语料跑一次 .docx → .doc，验证转换链是否真的可用。
func verifyConverterChain(cfg *config.Config) error {
	src := filepath.Join(cfg.Root, "converter-check-source.docx")
	defer os.Remove(src)
	// 用最小合法 docx（安装包里没有测试语料，这里现场构造一段最小 OOXML）
	body, err := converter.BuildMinimalDocx()
	if err != nil {
		return err
	}
	if err := os.WriteFile(src, body, 0o644); err != nil {
		return err
	}
	conv := converter.New(filepath.Join(cfg.Root, "convert"))
	if !conv.Available() {
		return errors.New("没有可用的转换通道，无法验证")
	}
	out := filepath.Join(cfg.Root, "converter-check.doc")
	defer os.Remove(out)
	if err := conv.ToDoc(context.Background(), src, out); err != nil {
		return fmt.Errorf("转换验证失败：%w", err)
	}
	head := make([]byte, 8)
	f, err := os.Open(out)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := io.ReadFull(f, head); err != nil {
		return err
	}
	fmt.Printf("转换验证通过：%s（文件头 % x）\n", filepath.Base(out), head)
	return nil
}

// inspectTemplate 打印模板的占位符与校验结论；存在问题（未知/未闭合/被打断）时返回错误。
func inspectTemplate(path string) error {
	fields := contract.FieldNames()
	res, err := docx.Inspect(path, fields)
	if err != nil {
		return err
	}
	out := map[string]any{
		"file":       path,
		"parts":      res.Parts,
		"fieldUsage": res.FieldUsage,
		"usedFields": res.UsedFields(),
		"missing":    res.Missing,
		"problems":   res.Problems(),
	}
	buf, _ := json.MarshalIndent(out, "", "  ")
	fmt.Println(string(buf))
	if problems := res.Problems(); len(problems) > 0 {
		return fmt.Errorf("模板存在 %d 个问题", len(problems))
	}
	return nil
}

// resolveConfig 决定数据目录：优先命令行覆盖，其次环境变量，最后系统用户级目录。
func resolveConfig(override string) (*config.Config, error) {
	if override == "" {
		override = os.Getenv("INSPIRATION_DATA_DIR")
	}
	if override != "" {
		return config.At(override)
	}
	return config.Default()
}
