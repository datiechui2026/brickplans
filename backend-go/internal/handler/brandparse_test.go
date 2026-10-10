package handler

import "testing"

func TestParseBrandFromTitle(t *testing.T) {
	cases := []struct {
		name      string
		title     string
		wantBrand string
		wantCode  string
		wantOK    bool
	}{
		{"lego english", "钢铁侠 Mark 3 收藏版 (LEGO 76344)", "乐高", "76344", true},
		{"lego upper", "Iron Man Mark 3 Collectors' Edition (LEGO 76344)", "乐高", "76344", true},
		{"lego registered sign", "林间蘑菇 (LEGO® 11505)", "乐高", "11505", true},
		{"fullwidth parens chinese brand", "某款场景（森宝 10236）", "森宝", "10236", true},
		{"chinese brand ascii parens", "蒸汽机车 (双鹰 88001)", "双鹰", "88001", true},
		{"code only", "无品牌图纸 (76344)", "", "76344", true},
		{"dash code", "越野车 (LEGO 60183-1)", "乐高", "60183-1", true},
		{"last group wins", "旧的 (森宝 11111) 新的 (LEGO 22222)", "乐高", "22222", true},
		{"trailing extra code ignored", "大黄蜂 (LEGO 10338 76288)", "乐高", "10338", true},
		{"lowercase lego", "Foo Bar (lego 76344)", "乐高", "76344", true},
		{"no parens", "纯粹的中文标题", "", "", false},
		{"non code parens", "限量版本 (仅限首发)", "", "", false},
		{"short digits not a code", "第二季 (第2季)", "", "", false},
		{"empty parens", "空括号 ()", "", "", false},
		{"prefix letters preserved", "机甲 (MOC 10236)", "MOC", "10236", true},
		// Real production titles: full-width nested volume label after the code.
		{"nested fullwidth volume", "新型远程战略轰炸机（下册） (奇积 23025（下册）)", "奇积", "23025", true},
		{"nested fullwidth volume 2", "歼-16 (奇积 23041（下册）)", "奇积", "23041", true},
		{"nested ascii volume", "某套件 (LEGO 76344 (Vol.2))", "乐高", "76344", true},
		{"chinese title with colon", "指环王：夏尔郡 (LEGO 10354)", "乐高", "10354", true},
		{"latin model name leftovers", "Polaroid OneStep SX-70 相机 (LEGO 21345)", "乐高", "21345", true},
		{"brand only chinese", "机甲小队首领 (LEGO 77078)", "乐高", "77078", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			brand, code, ok := parseBrandFromTitle(tc.title)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v (title=%q)", ok, tc.wantOK, tc.title)
			}
			if brand != tc.wantBrand || code != tc.wantCode {
				t.Fatalf("got (%q, %q), want (%q, %q)", brand, code, tc.wantBrand, tc.wantCode)
			}
		})
	}
}
