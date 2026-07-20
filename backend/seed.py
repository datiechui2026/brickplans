#!/usr/bin/env python3
"""Seed script for BrickPlans — pre-populate database with sample data.

Idempotent: re-running won't duplicate entries.
Usage: cd backend && python seed.py
"""

import os
import sys
import uuid
from datetime import datetime, timezone

# Ensure we can import app modules from backend/
sys.path.insert(0, os.path.dirname(__file__))

from sqlalchemy import create_engine, select, func
from sqlalchemy.orm import Session

from app.core.database import Base
from app.core.security import hash_password
from app.models import User, Blueprint

# ── Config ──────────────────────────────────────────────────────
DATABASE_URL = os.getenv("DATABASE_URL_SYNC", "sqlite:///./brickplans.db")
OFFICIAL_EMAIL = "official@brickplan.cn"
OFFICIAL_USERNAME = "BrickPlans官方"
OFFICIAL_PASSWORD = os.getenv("SEED_ADMIN_PASSWORD", "brickplans2024")
OFFICIAL_BIO = "BrickPlans 官方账号，分享高质量的积木图纸与创意灵感。"

# ── Seed Data ───────────────────────────────────────────────────
SEED_BLUEPRINTS = [
    # === 建筑 (5) ===
    {"title": "中世纪城堡", "category": "建筑", "difficulty": 4, "piece_count": 3200,
     "description": "经典的欧式中世纪城堡，包含城墙、塔楼和吊桥。主体使用灰色砖块搭建，细节部分用深灰和棕色点缀，适合中高级玩家挑战。"},
    {"title": "现代别墅", "category": "建筑", "difficulty": 3, "piece_count": 1800,
     "description": "极简风格的现代别墅设计，大面积玻璃幕墙搭配白色外墙，配有小花园和泳池。线条干净利落，非常适合作为城市街景的一部分。"},
    {"title": "日式神社", "category": "建筑", "difficulty": 3, "piece_count": 1500,
     "description": "传统日式神社建筑，红色鸟居、飞檐屋顶和石灯笼一应俱全。配色以红色、深棕和白色为主，还原度高。"},
    {"title": "太空基地", "category": "建筑", "difficulty": 5, "piece_count": 4500,
     "description": "月球表面的科研基地，包含主控室、生活舱、实验舱和发射台。大量使用白色和浅灰零件，配合透明蓝色零件模拟能源装置。"},
    {"title": "树屋村落", "category": "建筑", "difficulty": 2, "piece_count": 1200,
     "description": "建在巨型树木上的温馨树屋群落，由吊桥连接各个小屋。棕色和绿色为主色调，适合入门到中级玩家。"},

    # === 车辆 (5) ===
    {"title": "F1方程式赛车", "category": "车辆", "difficulty": 3, "piece_count": 950,
     "description": "高度还原的F1赛车模型，包含空气动力学套件、可转动方向盘和可拆卸引擎盖。红色涂装，细节丰富。"},
    {"title": "重型卡车", "category": "车辆", "difficulty": 4, "piece_count": 2200,
     "description": "美式重型卡车头，配有可开启车门、可转向前轮和精致的驾驶室内饰。经典的红蓝配色。"},
    {"title": "复古摩托车", "category": "车辆", "difficulty": 2, "piece_count": 450,
     "description": "经典Cafe Racer风格摩托车，棕色皮质座椅、圆形大灯和链条传动。线条流畅，适合展示。"},
    {"title": "挖掘机工程车", "category": "车辆", "difficulty": 3, "piece_count": 1600,
     "description": "功能性挖掘机模型，液压臂可上下活动，铲斗可开合，履带可转动。黄色为主，黑灰点缀。"},
    {"title": "双层观光巴士", "category": "车辆", "difficulty": 2, "piece_count": 800,
     "description": "伦敦经典红色双层巴士，上下层内部均有座椅细节，车身广告贴纸可自定义。城市街景必备。"},

    # === 机甲 (5) ===
    {"title": "重型突击机甲", "category": "机甲", "difficulty": 5, "piece_count": 3800,
     "description": "武装到牙齿的重型机甲，双肩搭载导弹发射器，右手持等离子炮，左手能量盾。关节可动，姿势丰富。"},
    {"title": "忍者机甲", "category": "机甲", "difficulty": 3, "piece_count": 1200,
     "description": "轻量化的忍者型机甲，配备双刀和手里剑，背部推进器可展开。黑红配色，灵活帅气。"},
    {"title": "蒸汽朋克机器人", "category": "机甲", "difficulty": 4, "piece_count": 2100,
     "description": "维多利亚风格的蒸汽动力机器人，黄铜色齿轮、管道和压力表细节满满。左手为多功能工具臂。"},
    {"title": "动物合体机甲-狮王", "category": "机甲", "difficulty": 4, "piece_count": 2600,
     "description": "以雄狮为原型的合体机甲，头部鬃毛展开后露出武器阵列，四爪可变换形态。橙金配色，霸气十足。"},
    {"title": "微型侦察机甲", "category": "机甲", "difficulty": 1, "piece_count": 350,
     "description": "紧凑型侦察机甲，身材小巧但细节不缩水。可动关节多，适合桌面展示，新手友好。"},

    # === 奇幻 (3) ===
    {"title": "巨龙巢穴", "category": "奇幻", "difficulty": 5, "piece_count": 3500,
     "description": "盘踞在宝藏堆上的红龙，双翼展开超过40cm，嘴里可喷出透明橙色火焰特效件。龙鳞层次分明。"},
    {"title": "独角兽森林", "category": "奇幻", "difficulty": 2, "piece_count": 800,
     "description": "在魔法森林中漫步的独角兽，彩色鬃毛和尾巴使用渐变零件，角为金色。搭配发光蘑菇和小精灵场景。"},
    {"title": "矮人铁匠铺", "category": "奇幻", "difficulty": 3, "piece_count": 1300,
     "description": "山体中的矮人工坊，包含锻造台、淬火池和武器展示架。暖色灯光效果让整个场景充满温度。"},

    # === 科幻 (4) ===
    {"title": "星际战舰", "category": "科幻", "difficulty": 5, "piece_count": 5000,
     "description": "旗舰级星际战列舰，流线型舰体搭配粒子炮阵列，舰桥和引擎细节精致。银灰配色，科幻感十足。"},
    {"title": "赛博朋克街道", "category": "科幻", "difficulty": 4, "piece_count": 2800,
     "description": "霓虹灯闪烁的未来都市街角，包含拉面摊、全息广告牌和飞行汽车。紫色和青色为主色调，氛围拉满。"},
    {"title": "太空电梯", "category": "科幻", "difficulty": 3, "piece_count": 1600,
     "description": "连接地面与同步轨道的太空电梯，缆绳使用透明零件模拟，底部为发射平台。科幻设定的标志性建筑。"},
    {"title": "机器人宠物店", "category": "科幻", "difficulty": 2, "piece_count": 700,
     "description": "贩卖各式机器宠物的街边小店，橱窗里展示着机器狗、机器猫和机器鸟。温馨可爱的未来日常。"},

    # === 场景 (3) ===
    {"title": "海盗湾", "category": "场景", "difficulty": 4, "piece_count": 2500,
     "description": "热带海岛上的海盗据点，包含海盗船残骸改造的酒馆、藏宝洞穴和瞭望塔。蓝海白沙棕木的经典搭配。"},
    {"title": "深夜便利店", "category": "场景", "difficulty": 1, "piece_count": 400,
     "description": "24小时营业的便利店，货架、收银台和冷饮柜一应俱全。暖黄色灯光透过玻璃窗，充满城市夜晚的孤独感。"},
    {"title": "迷你游乐场", "category": "场景", "difficulty": 3, "piece_count": 2000,
     "description": "拥有摩天轮、旋转木马和过山车的迷你游乐场，每个设施都可以手动转动。五彩缤纷，亲子互动好选择。"},
]


def slugify(title: str) -> str:
    """Simple slug — matching the backend logic."""
    # Basic slug: lowercase, replace spaces with hyphens
    slug = title.lower().replace(' ', '-')
    # Add suffix to avoid conflicts
    return f"{slug}-official"


def seed(conn_str: str = DATABASE_URL):
    engine = create_engine(conn_str, echo=False)
    Base.metadata.create_all(engine)

    with Session(engine) as session:
        # ── Check idempotent ──
        count = session.execute(select(func.count()).select_from(Blueprint)).scalar()
        if count >= 10:
            print(f"[SKIP] Database already has {count} blueprints — nothing to seed.")
            return

        # ── Create official user ──
        existing = session.execute(
            select(User).where(User.email == OFFICIAL_EMAIL)
        ).scalar_one_or_none()

        if existing:
            official = existing
            print(f"[INFO] Official user already exists: {official.username} (id={official.id})")
        else:
            official = User(
                id=str(uuid.uuid4()),
                username=OFFICIAL_USERNAME,
                email=OFFICIAL_EMAIL,
                password_hash=hash_password(OFFICIAL_PASSWORD),
                avatar_url=None,
                bio=OFFICIAL_BIO,
                created_at=datetime.now(timezone.utc),
            )
            session.add(official)
            session.flush()
            print(f"[OK] Created official user: {official.username} (id={official.id})")

        # ── Insert blueprints ──
        inserted = 0
        for item in SEED_BLUEPRINTS:
            slug = slugify(item["title"])
            existing_bp = session.execute(
                select(Blueprint).where(Blueprint.slug == slug)
            ).scalar_one_or_none()
            if existing_bp:
                continue

            bp = Blueprint(
                id=str(uuid.uuid4()),
                author_id=official.id,
                title=item["title"],
                slug=slug,
                description=item["description"],
                difficulty=item["difficulty"],
                piece_count=item["piece_count"],
                category=item["category"],
                dimensions=None,
                part_list=None,
                view_count=0,
                is_published=True,
                created_at=datetime.now(timezone.utc),
                updated_at=datetime.now(timezone.utc),
            )
            session.add(bp)
            inserted += 1

        session.commit()

        # ── Summary ──
        total = session.execute(select(func.count()).select_from(Blueprint)).scalar()
        per_cat = session.execute(
            select(Blueprint.category, func.count())
            .where(Blueprint.is_published == True)
            .group_by(Blueprint.category)
        ).all()

        print(f"\n═══ Seed Complete ═══")
        print(f"Inserted: {inserted} new blueprints")
        print(f"Total blueprints: {total}")
        print(f"By category:")
        for cat, cnt in per_cat:
            print(f"  {cat or 'uncategorized'}: {cnt}")


if __name__ == "__main__":
    seed()
