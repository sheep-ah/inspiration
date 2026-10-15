package updater

import (
	"testing"
)

func TestCompareVersions(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"1.0.0", "1.0.0", 0},
		{"v1.0.0", "1.0.0", 0},
		{"1.0.1", "1.0.0", 1},
		{"1.0.0", "1.0.1", -1},
		{"1.10.0", "1.9.9", 1},
		{"2.0.0", "1.99.99", 1},
		{"1.0.0", "1.0", 0},      // 缺省段按 0
		{"dev-abc", "1.0.0", -1}, // 非数字按 0 → 0.0.0 < 1.0.0
		{"1.0.0-rc.1", "1.0.0", -1},
		{"1.0.0", "1.0.0-rc.1", 1},
		{"1.0.0-rc.1", "1.0.0-rc.2", -1},
		{"1.0.0-rc.10", "1.0.0-rc.9", 1},
		{"1.0.0-beta.1", "1.0.0-rc.1", -1},
	}
	for _, c := range cases {
		if got := CompareVersions(c.a, c.b); got != c.want {
			t.Errorf("CompareVersions(%q,%q) = %d，期望 %d", c.a, c.b, got, c.want)
		}
	}
}

func TestIsNewer(t *testing.T) {
	if !IsNewer("1.0.0", "v1.0.1") {
		t.Error("1.0.0 → v1.0.1 应判定为新版本")
	}
	if IsNewer("1.0.0", "1.0.0") {
		t.Error("相同版本不应判定为新版本")
	}
	if IsNewer("1.1.0", "1.0.9") {
		t.Error("更旧版本不应判定为新版本")
	}
	if IsNewer("1.0.0", "") {
		t.Error("空版本不应判定为新版本")
	}
}

func TestValidateTag(t *testing.T) {
	for _, ok := range []string{"v1.0.0", "1.2.3", "v10.20.30"} {
		if err := validateTag(ok); err != nil {
			t.Errorf("%q 应通过校验：%v", ok, err)
		}
	}
	for _, bad := range []string{"v1.2", "1.2.3-rc1", "vabc", "", "v1.2.3.4"} {
		if err := validateTag(bad); err == nil {
			t.Errorf("%q 应被拒绝", bad)
		}
	}
}

func TestPickAssetPerPlatform(t *testing.T) {
	assets := []Asset{
		{Name: "inspiration_1.0.0_windows_amd64_setup.exe"},
		{Name: "inspiration_1.0.0_windows_amd64_portable.exe"},
		{Name: "inspiration_1.0.0_darwin_arm64.zip"},
		{Name: "inspiration_1.0.0_darwin_arm64.dmg"},
		{Name: "inspiration_1.0.0_darwin_amd64.zip"},
		{Name: "inspiration_1.0.0_darwin_amd64.dmg"},
		{Name: "checksums.txt"},
	}
	cases := map[[2]string]string{
		{"windows", "amd64"}: "inspiration_1.0.0_windows_amd64_setup.exe",
		{"darwin", "arm64"}:  "inspiration_1.0.0_darwin_arm64.zip",
		{"darwin", "amd64"}:  "inspiration_1.0.0_darwin_amd64.zip",
	}
	for k, want := range cases {
		got, err := pickAsset(assets, k[0], k[1])
		if err != nil {
			t.Fatalf("%s/%s 选择资产失败：%v", k[0], k[1], err)
		}
		if got.Name != want {
			t.Errorf("%s/%s 选中 %s，期望 %s", k[0], k[1], got.Name, want)
		}
	}

	// macOS 缺 zip 时退回 dmg；完全缺失时报错而不是随便挑一个
	onlyDMG := []Asset{{Name: "inspiration_1.0.0_darwin_arm64.dmg"}}
	if a, err := pickAsset(onlyDMG, "darwin", "arm64"); err != nil || a.Name != onlyDMG[0].Name {
		t.Errorf("缺 zip 时应退回 dmg：%v %v", a, err)
	}
	if _, err := pickAsset(onlyDMG, "windows", "amd64"); err == nil {
		t.Error("平台完全没有资产时应当报错")
	}
}

func TestCheckHostRejectsUntrustedSource(t *testing.T) {
	ok := []string{
		"https://github.com/sheep-ah/inspiration/releases/download/v1.0.0/a.zip",
		"https://objects.githubusercontent.com/github-production-release-asset/1/2",
		"https://release-assets.githubusercontent.com/x",
	}
	for _, u := range ok {
		if err := checkHost(u); err != nil {
			t.Errorf("%s 应被接受：%v", u, err)
		}
	}
	bad := []string{
		"http://github.com/a.zip",               // 非 HTTPS
		"https://evil.example.com/a.zip",        // 非 GitHub
		"https://github.com.evil.example/a.zip", // 后缀伪装
		"file:///etc/passwd",
	}
	for _, u := range bad {
		if err := checkHost(u); err == nil {
			t.Errorf("%s 应被拒绝", u)
		}
	}
}

func TestParseChecksums(t *testing.T) {
	text := `d0cf11e0a1b11ae1d0cf11e0a1b11ae1d0cf11e0a1b11ae1d0cf11e0a1b11ae1  inspiration_1.0.0_windows_amd64_setup.exe
0000000000000000000000000000000000000000000000000000000000000000 *inspiration_1.0.0_darwin_arm64.zip
not-a-hash  something.txt
short  33  
`
	sums := ParseChecksums(text)
	if len(sums) != 2 {
		t.Fatalf("应解析出 2 条有效校验和，实际 %d：%v", len(sums), sums)
	}
	if sums["inspiration_1.0.0_windows_amd64_setup.exe"] != "d0cf11e0a1b11ae1d0cf11e0a1b11ae1d0cf11e0a1b11ae1d0cf11e0a1b11ae1" {
		t.Error("第一条校验和解析错误")
	}
	if _, ok := sums["inspiration_1.0.0_darwin_arm64.zip"]; !ok {
		t.Error("带 * 前缀的文件名应被正确解析")
	}
}

func TestVerifySHA256(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/file.bin"
	// "hello" 的 sha256
	const want = "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"
	if err := writeFile(path, "hello"); err != nil {
		t.Fatal(err)
	}
	if err := VerifySHA256(path, want); err != nil {
		t.Errorf("正确校验和应通过：%v", err)
	}
	if err := VerifySHA256(path, "sha256:"+want); err != nil {
		t.Errorf("带 sha256: 前缀的校验和应通过：%v", err)
	}
	// 校验失败必须删除文件，避免被误安装
	if err := writeFile(path, "hello"); err != nil {
		t.Fatal(err)
	}
	if err := VerifySHA256(path, "1111111111111111111111111111111111111111111111111111111111111111"); err == nil {
		t.Error("错误校验和必须报错")
	}
	if fileExists(path) {
		t.Error("校验失败的文件必须被删除")
	}
	// 缺少校验和也要拒绝
	if err := writeFile(path, "hello"); err != nil {
		t.Fatal(err)
	}
	if err := VerifySHA256(path, ""); err == nil {
		t.Error("缺少校验和时必须拒绝安装")
	}
}
