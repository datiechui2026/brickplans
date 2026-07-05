// Command seed pre-populates the MySQL database with the official account and a
// set of sample blueprints. Idempotent: re-running won't duplicate entries.
//
// Usage: cd backend-go && go run ./cmd/seed
package main

import (
	"log"
	"os"
	"strings"

	"brickplans/internal/auth"
	"brickplans/internal/config"
	"brickplans/internal/db"
)

var (
	nonAlphaNumDash = strings.NewReplacer(" ", "-")
)

type seedBP struct {
	Title       string
	Category    string
	Difficulty  int
	PieceCount  int
	Description string
}

var seedBlueprints = []seedBP{
	{"中世纪城堡", "建筑", 4, 3200, "经典的欧式中世纪城堡，包含城墙、塔楼和吊桥。主体使用灰色砖块搭建，细节部分用深灰和棕色点缀，适合中高级玩家挑战。"},
	{"现代别墅", "建筑", 3, 1800, "极简风格的现代别墅设计，大面积玻璃幕墙搭配白色外墙，配有小花园和泳池。线条干净利落，非常适合作为城市街景的一部分。"},
	{"日式神社", "建筑", 3, 1500, "传统日式神社建筑，红色鸟居、飞檐屋顶和石灯笼一应俱全。配色以红色、深棕和白色为主，还原度高。"},
	{"太空基地", "建筑", 5, 4500, "月球表面的科研基地，包含主控室、生活舱、实验舱和发射台。大量使用白色和浅灰零件，配合透明蓝色零件模拟能源装置。"},
	{"树屋村落", "建筑", 2, 1200, "建在巨型树木上的温馨树屋群落，由吊桥连接各个小屋。棕色和绿色为主色调，适合入门到中级玩家。"},
	{"F1方程式赛车", "车辆", 3, 950, "高度还原的F1赛车模型，包含空气动力学套件、可转动方向盘和可拆卸引擎盖。红色涂装，细节丰富。"},
	{"重型卡车", "车辆", 4, 2200, "美式重型卡车头，配有可开启车门、可转向前轮和精致的驾驶室内饰。经典的红蓝配色。"},
	{"复古摩托车", "车辆", 2, 450, "经典Cafe Racer风格摩托车，棕色皮质座椅、圆形大灯和链条传动。线条流畅，适合展示。"},
	{"挖掘机工程车", "车辆", 3, 1600, "功能性挖掘机模型，液压臂可上下活动，铲斗可开合，履带可转动。黄色为主，黑灰点缀。"},
	{"双层观光巴士", "车辆", 2, 800, "伦敦经典红色双层巴士，上下层内部均有座椅细节，车身广告贴纸可自定义。城市街景必备。"},
	{"重型突击机甲", "机甲", 5, 3800, "武装到牙齿的重型机甲，双肩搭载导弹发射器，右手持等离子炮，左手能量盾。关节可动，姿势丰富。"},
	{"忍者机甲", "机甲", 3, 1200, "轻量化的忍者型机甲，配备双刀和手里剑，背部推进器可展开。黑红配色，灵活帅气。"},
	{"蒸汽朋克机器人", "机甲", 4, 2100, "维多利亚风格的蒸汽动力机器人，黄铜色齿轮、管道和压力表细节满满。左手为多功能工具臂。"},
	{"动物合体机甲-狮王", "机甲", 4, 2600, "以雄狮为原型的合体机甲，头部鬃毛展开后露出武器阵列，四爪可变换形态。橙金配色，霸气十足。"},
	{"微型侦察机甲", "机甲", 1, 350, "紧凑型侦察机甲，身材小巧但细节不缩水。可动关节多，适合桌面展示，新手友好。"},
	{"巨龙巢穴", "奇幻", 5, 3500, "盘踞在宝藏堆上的红龙，双翼展开超过40cm，嘴里可喷出透明橙色火焰特效件。龙鳞层次分明。"},
	{"独角兽森林", "奇幻", 2, 800, "在魔法森林中漫步的独角兽，彩色鬃毛和尾巴使用渐变零件，角为金色。搭配发光蘑菇和小精灵场景。"},
	{"矮人铁匠铺", "奇幻", 3, 1300, "山体中的矮人工坊，包含锻造台、淬火池和武器展示架。暖色灯光效果让整个场景充满温度。"},
	{"星际战舰", "科幻", 5, 5000, "旗舰级星际战列舰，流线型舰体搭配粒子炮阵列，舰桥和引擎细节精致。银灰配色，科幻感十足。"},
	{"赛博朋克街道", "科幻", 4, 2800, "霓虹灯闪烁的未来都市街角，包含拉面摊、全息广告牌和飞行汽车。紫色和青色为主色调，氛围拉满。"},
	{"太空电梯", "科幻", 3, 1600, "连接地面与同步轨道的太空电梯，缆绳使用透明零件模拟，底部为发射平台。科幻设定的标志性建筑。"},
	{"机器人宠物店", "科幻", 2, 700, "贩卖各式机器宠物的街边小店，橱窗里展示着机器狗、机器猫和机器鸟。温馨可爱的未来日常。"},
	{"海盗湾", "场景", 4, 2500, "热带海岛上的海盗据点，包含海盗船残骸改造的酒馆、藏宝洞穴和瞭望塔。蓝海白沙棕木的经典搭配。"},
	{"深夜便利店", "场景", 1, 400, "24小时营业的便利店，货架、收银台和冷饮柜一应俱全。暖黄色灯光透过玻璃窗，充满城市夜晚的孤独感。"},
	{"迷你游乐场", "场景", 3, 2000, "拥有摩天轮、旋转木马和过山车的迷你游乐场，每个设施都可以手动转动。五彩缤纷，亲子互动好选择。"},
}

func slugify(title string) string {
	s := strings.ToLower(strings.TrimSpace(title))
	s = nonAlphaNumDash.Replace(s)
	return s + "-official"
}

func main() {
	cfg := config.Load()
	gdb, err := db.Open(cfg.MySQLDSN, cfg.AppEnv)
	if err != nil {
		log.Fatalf("db connect: %v", err)
	}
	if err := gdb.AutoMigrate(db.AllModels()...); err != nil {
		log.Fatalf("auto-migrate: %v", err)
	}

	var count int64
	gdb.Model(&db.Blueprint{}).Count(&count)
	if count >= 10 {
		log.Printf("[SKIP] database already has %d blueprints — nothing to seed.", count)
		return
	}

	officialEmail := getenv("SEED_OFFICIAL_EMAIL", "official@brickplans.com")
	officialUsername := getenv("SEED_OFFICIAL_USERNAME", "BrickPlans官方")
	officialPassword := getenv("SEED_ADMIN_PASSWORD", "brickplans2024")
	officialBio := "BrickPlans 官方账号，分享高质量的积木图纸与创意灵感。"

	var official db.User
	if gdb.Where("email = ?", officialEmail).First(&official).Error != nil {
		hash, err := auth.HashPassword(officialPassword)
		if err != nil {
			log.Fatalf("hash: %v", err)
		}
		official = db.User{Username: officialUsername, Email: officialEmail, PasswordHash: hash, Bio: &officialBio}
		if err := gdb.Create(&official).Error; err != nil {
			log.Fatalf("create official user: %v", err)
		}
		log.Printf("[OK] created official user: %s (id=%s)", official.Username, official.ID)
	} else {
		log.Printf("[INFO] official user exists: %s (id=%s)", official.Username, official.ID)
	}

	inserted := 0
	for _, item := range seedBlueprints {
		slug := slugify(item.Title)
		var existing db.Blueprint
		if gdb.Where("slug = ?", slug).First(&existing).Error == nil {
			continue
		}
		bp := db.Blueprint{
			Title:       item.Title,
			Slug:        slug,
			Description: &item.Description,
			Difficulty:  &item.Difficulty,
			PieceCount:  &item.PieceCount,
			Category:    &item.Category,
			IsPublished: true,
			AuthorID:    official.ID,
		}
		if err := gdb.Create(&bp).Error; err != nil {
			log.Printf("[WARN] create %q: %v", item.Title, err)
			continue
		}
		inserted++
	}

	var total int64
	gdb.Model(&db.Blueprint{}).Count(&total)
	log.Printf("════ Seed complete ════ inserted=%d total=%d", inserted, total)
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
