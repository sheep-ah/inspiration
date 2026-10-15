package contract

import (
	"strings"
	"testing"
	"time"
)

func validValues() Values {
	return Values{
		"联系人姓名":  "张三",
		"法人代表":   "李四",
		"实际经营地址": "北京市海淀区中关村大街1号院3号楼5层501室",
		"联系人邮箱":  "zhangsan@example.com",
		"营业执照名称": "示例科技（北京）有限公司",
		"纳税人识别号": "91110108MA01ABCD2X",
		"电话":     "010-88886666",
		"联系人电话":  "13800000000",
		"开户银行":   "中国工商银行股份有限公司北京中关村支行",
		"账户号":    "0001234567890123456",
		"开票地址":   "上海市浦东新区世纪大道100号A座20层",
	}
}

func TestFieldNamesMatchPlaceholderContract(t *testing.T) {
	want := []string{"联系人姓名", "法人代表", "实际经营地址", "联系人邮箱", "营业执照名称", "纳税人识别号", "电话", "联系人电话", "开户银行", "账户号", "开票地址"}
	got := FieldNames()
	if len(got) != len(want) {
		t.Fatalf("字段数量 = %d，期望 %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("字段 %d = %q，期望 %q", i, got[i], want[i])
		}
	}
}

func TestValidateAcceptsCompleteForm(t *testing.T) {
	if errs := validValues().Validate(); len(errs) != 0 {
		t.Fatalf("完整表单不应报错: %+v", errs)
	}
}

func TestValidateRequiresEveryField(t *testing.T) {
	for _, f := range Fields {
		values := validValues()
		values[f.Key] = "   " // 只有空白也应视为未填写
		errs := values.Validate()
		if len(errs) == 0 {
			t.Errorf("字段 %s 为空时必须报错", f.Key)
			continue
		}
		found := false
		for _, e := range errs {
			if e.Key == f.Key {
				found = true
			}
		}
		if !found {
			t.Errorf("字段 %s 为空时错误未指向该字段: %+v", f.Key, errs)
		}
	}
}

func TestValidateEmailFormat(t *testing.T) {
	bad := []string{"not-an-email", "a@b", "a@b.", "@example.com", "张三@example.com", "a b@example.com", "a@@example.com"}
	for _, email := range bad {
		values := validValues()
		values["联系人邮箱"] = email
		errs := values.Validate()
		hit := false
		for _, e := range errs {
			if e.Key == "联系人邮箱" {
				hit = true
			}
		}
		if !hit {
			t.Errorf("邮箱 %q 应被判为格式错误", email)
		}
	}
	good := []string{"a@example.com", "first.last+tag@sub.example.co", "zhangsan@example.com.cn"}
	for _, email := range good {
		values := validValues()
		values["联系人邮箱"] = email
		for _, e := range values.Validate() {
			if e.Key == "联系人邮箱" {
				t.Errorf("邮箱 %q 不应被判为格式错误：%s", email, e.Message)
			}
		}
	}
}

func TestValidateRejectsOverlongValue(t *testing.T) {
	values := validValues()
	values["联系人姓名"] = strings.Repeat("张", 200)
	errs := values.Validate()
	if len(errs) == 0 {
		t.Fatal("超长字段应当报错")
	}
}

func TestValidateShortTaxID(t *testing.T) {
	values := validValues()
	values["纳税人识别号"] = "1234567890123" // 13 位
	hit := false
	for _, e := range values.Validate() {
		if e.Key == "纳税人识别号" {
			hit = true
		}
	}
	if !hit {
		t.Error("过短的纳税人识别号应当报错")
	}
}

func TestValuesFromMapKeepsLeadingZerosAndTrims(t *testing.T) {
	raw := map[string]string{
		"账户号":  "  0001234567890123456  ",
		"电话":   " 010-88886666 ",
		"未知字段": "应被忽略",
	}
	values := ValuesFromMap(raw)
	if values["账户号"] != "0001234567890123456" {
		t.Errorf("账户号前导零被破坏: %q", values["账户号"])
	}
	if values["电话"] != "010-88886666" {
		t.Errorf("电话未正确 trim: %q", values["电话"])
	}
	if _, exists := values["未知字段"]; exists {
		t.Error("未知字段应被忽略")
	}
}

func TestDefaultFileName(t *testing.T) {
	values := validValues()
	name := DefaultFileName(values)
	if !strings.HasPrefix(name, "合同-示例科技（北京）有限公司-") {
		t.Errorf("默认文件名不符合预期: %s", name)
	}
	if !strings.HasSuffix(name, time.Now().Format("20060102")) {
		t.Errorf("默认文件名应包含当天日期: %s", name)
	}
}

func TestDefaultFileNameFallsBackWhenCompanyMissing(t *testing.T) {
	name := DefaultFileName(Values{})
	if !strings.HasPrefix(name, "合同-") {
		t.Errorf("缺少公司名时文件名应有兜底: %s", name)
	}
}
