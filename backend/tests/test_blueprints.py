"""
图纸 CRUD 测试 (RED 阶段 — 测试先于实现)

测试覆盖：
- 创建图纸 (认证/未认证)
- 获取图纸详情
- 更新图纸 (作者/非作者)
- 删除图纸 (作者/非作者)
- 列表分页 + 搜索 + 筛选
"""
import pytest
from httpx import AsyncClient


class TestCreateBlueprint:
    """图纸创建"""

    async def test_create_blueprint_requires_auth(self, client: AsyncClient):
        """RED: 未认证用户创建图纸应返回 401"""
        response = await client.post("/api/blueprints", json={
            "title": "My MOC",
            "description": "A cool build",
        })
        assert response.status_code == 401

    async def test_create_blueprint_succeeds(self, client: AsyncClient):
        """RED: 认证用户创建图纸应返回 201"""
        token = await _register_and_login(client, "creator", "creator@test.com")
        headers = {"Authorization": f"Bearer {token}"}

        response = await client.post("/api/blueprints", json={
            "title": "My First MOC",
            "description": "A spaceship made of 200 pieces",
            "difficulty": 3,
            "piece_count": 200,
            "category": "vehicles",
            "dimensions": "30x20x15 cm",
            "part_list": {"bricks": 150, "plates": 30, "slopes": 20},
        }, headers=headers)

        assert response.status_code == 201
        data = response.json()
        assert data["title"] == "My First MOC"
        assert data["slug"] == "my-first-moc"
        assert data["author"]["username"] == "creator"
        assert data["difficulty"] == 3
        assert data["piece_count"] == 200

    async def test_create_without_title_fails(self, client: AsyncClient):
        """RED: 缺少标题应返回 422"""
        token = await _register_and_login(client, "badt", "badt@test.com")
        headers = {"Authorization": f"Bearer {token}"}

        response = await client.post("/api/blueprints", json={
            "description": "Missing title",
        }, headers=headers)
        assert response.status_code == 422


class TestGetBlueprint:
    """图纸获取"""

    async def test_get_blueprint_by_id(self, client: AsyncClient):
        """RED: 按 ID 获取图纸详情"""
        token = await _register_and_login(client, "getter", "getter@test.com")
        headers = {"Authorization": f"Bearer {token}"}

        create_resp = await client.post("/api/blueprints", json={
            "title": "Retrievable MOC",
            "description": "Test blueprint",
        }, headers=headers)
        bp_id = create_resp.json()["id"]

        response = await client.get(f"/api/blueprints/{bp_id}")
        assert response.status_code == 200
        assert response.json()["title"] == "Retrievable MOC"
        assert response.json()["view_count"] == 1  # view counted

    async def test_get_nonexistent_blueprint_returns_404(self, client: AsyncClient):
        """RED: 不存在的图纸返回 404"""
        response = await client.get("/api/blueprints/00000000-0000-0000-0000-000000000000")
        assert response.status_code == 404


class TestUpdateBlueprint:
    """图纸更新"""

    async def test_update_own_blueprint(self, client: AsyncClient):
        """RED: 作者可以更新自己的图纸"""
        token = await _register_and_login(client, "updater", "updater@test.com")
        headers = {"Authorization": f"Bearer {token}"}

        create_resp = await client.post("/api/blueprints", json={
            "title": "Old Title",
            "description": "Old desc",
        }, headers=headers)
        bp_id = create_resp.json()["id"]

        response = await client.put(f"/api/blueprints/{bp_id}", json={
            "title": "New Title",
            "description": "Updated description",
        }, headers=headers)
        assert response.status_code == 200
        assert response.json()["title"] == "New Title"

    async def test_update_others_blueprint_fails(self, client: AsyncClient):
        """RED: 非作者不能更新他人图纸"""
        token_a = await _register_and_login(client, "author_a", "a@test.com")
        headers_a = {"Authorization": f"Bearer {token_a}"}

        create_resp = await client.post("/api/blueprints", json={
            "title": "Author A's MOC",
            "description": "Mine",
        }, headers=headers_a)
        bp_id = create_resp.json()["id"]

        token_b = await _register_and_login(client, "intruder", "b@test.com")
        headers_b = {"Authorization": f"Bearer {token_b}"}

        response = await client.put(f"/api/blueprints/{bp_id}", json={
            "title": "Stolen!",
        }, headers=headers_b)
        assert response.status_code == 403


class TestDeleteBlueprint:
    """图纸删除"""

    async def test_delete_own_blueprint(self, client: AsyncClient):
        """RED: 作者可以删除自己的图纸"""
        token = await _register_and_login(client, "deleter", "deleter@test.com")
        headers = {"Authorization": f"Bearer {token}"}

        create_resp = await client.post("/api/blueprints", json={
            "title": "To Delete",
            "description": "Gone soon",
        }, headers=headers)
        bp_id = create_resp.json()["id"]

        response = await client.delete(f"/api/blueprints/{bp_id}", headers=headers)
        assert response.status_code == 204

        # 确认已删除
        get_resp = await client.get(f"/api/blueprints/{bp_id}")
        assert get_resp.status_code == 404

    async def test_delete_others_blueprint_fails(self, client: AsyncClient):
        """RED: 非作者不能删除他人图纸"""
        token_a = await _register_and_login(client, "owner", "owner@test.com")
        token_b = await _register_and_login(client, "hacker", "hacker@test.com")

        headers_a = {"Authorization": f"Bearer {token_a}"}
        create_resp = await client.post("/api/blueprints", json={
            "title": "Owner's MOC",
            "description": "Hands off",
        }, headers=headers_a)
        bp_id = create_resp.json()["id"]

        headers_b = {"Authorization": f"Bearer {token_b}"}
        response = await client.delete(f"/api/blueprints/{bp_id}", headers=headers_b)
        assert response.status_code == 403


class TestListBlueprints:
    """图纸列表 + 搜索 + 筛选"""

    async def test_list_blueprints_returns_paginated(self, client: AsyncClient):
        """RED: 列表返回分页结果"""
        token = await _register_and_login(client, "lister", "lister@test.com")
        headers = {"Authorization": f"Bearer {token}"}

        for i in range(5):
            await client.post("/api/blueprints", json={
                "title": f"Blueprint {i}",
                "description": f"Desc {i}",
            }, headers=headers)

        response = await client.get("/api/blueprints?page=1&size=3")
        assert response.status_code == 200
        data = response.json()
        assert data["total"] == 5
        assert len(data["items"]) == 3

    async def test_search_blueprints(self, client: AsyncClient):
        """RED: 搜索标题和描述"""
        token = await _register_and_login(client, "searcher", "searcher@test.com")
        headers = {"Authorization": f"Bearer {token}"}

        await client.post("/api/blueprints", json={"title": "Spaceship MOC", "description": "From Star Wars"}, headers=headers)
        await client.post("/api/blueprints", json={"title": "Castle MOC", "description": "Medieval fortress"}, headers=headers)
        await client.post("/api/blueprints", json={"title": "Rocket", "description": "Goes to space"}, headers=headers)

        # 搜索 "space"
        response = await client.get("/api/blueprints?q=space")
        assert response.status_code == 200
        data = response.json()
        assert data["total"] == 2  # Spaceship + Rocket (space in title or desc)

    async def test_filter_by_category(self, client: AsyncClient):
        """RED: 按分类筛选"""
        token = await _register_and_login(client, "filterer", "filterer@test.com")
        headers = {"Authorization": f"Bearer {token}"}

        await client.post("/api/blueprints", json={"title": "Car", "category": "vehicles"}, headers=headers)
        await client.post("/api/blueprints", json={"title": "House", "category": "architecture"}, headers=headers)
        await client.post("/api/blueprints", json={"title": "Truck", "category": "vehicles"}, headers=headers)

        response = await client.get("/api/blueprints?category=vehicles")
        assert response.status_code == 200
        data = response.json()
        assert data["total"] == 2

    async def test_sort_by_popularity(self, client: AsyncClient):
        """RED: 按热门排序 (view_count)"""
        token = await _register_and_login(client, "sorter", "sorter@test.com")
        headers = {"Authorization": f"Bearer {token}"}

        resp1 = await client.post("/api/blueprints", json={"title": "Popular"}, headers=headers)
        resp2 = await client.post("/api/blueprints", json={"title": "Hidden Gem"}, headers=headers)

        bp1_id = resp1.json()["id"]
        bp2_id = resp2.json()["id"]

        # 多次查看 Popular
        for _ in range(3):
            await client.get(f"/api/blueprints/{bp1_id}")

        response = await client.get("/api/blueprints?sort=popular")
        assert response.status_code == 200
        items = response.json()["items"]
        # "Popular" 应该在 "Hidden Gem" 前面
        assert items[0]["title"] == "Popular"


class TestFavoriteBlueprint:
    """收藏功能"""

    async def test_favorite_blueprint(self, client: AsyncClient):
        """RED: 收藏图纸并检查收藏状态"""
        author_token = await _register_and_login(client, "fav_author", "fav_author@test.com")
        fan_token = await _register_and_login(client, "fan", "fan@test.com")

        # 创建图纸
        headers_author = {"Authorization": f"Bearer {author_token}"}
        create_resp = await client.post("/api/blueprints", json={
            "title": "Favorite This",
            "description": "A masterpiece",
        }, headers=headers_author)
        bp_id = create_resp.json()["id"]

        # 收藏
        headers_fan = {"Authorization": f"Bearer {fan_token}"}
        fav_resp = await client.post(f"/api/blueprints/{bp_id}/favorite", headers=headers_fan)
        assert fav_resp.status_code == 201

        # 获取详情，应显示已收藏
        detail_resp = await client.get(f"/api/blueprints/{bp_id}", headers=headers_fan)
        assert detail_resp.json()["is_favorited"] is True
        assert detail_resp.json()["favorite_count"] == 1

    async def test_unfavorite_blueprint(self, client: AsyncClient):
        """RED: 取消收藏"""
        token = await _register_and_login(client, "unfaver", "unfaver@test.com")
        headers = {"Authorization": f"Bearer {token}"}

        create_resp = await client.post("/api/blueprints", json={
            "title": "Unfavorite Later",
            "description": "Temporary love",
        }, headers=headers)
        bp_id = create_resp.json()["id"]

        await client.post(f"/api/blueprints/{bp_id}/favorite", headers=headers)
        unfav_resp = await client.delete(f"/api/blueprints/{bp_id}/favorite", headers=headers)
        assert unfav_resp.status_code == 204

        detail_resp = await client.get(f"/api/blueprints/{bp_id}", headers=headers)
        assert detail_resp.json()["is_favorited"] is False

    async def test_favorite_requires_auth(self, client: AsyncClient):
        """RED: 未认证不能收藏"""
        response = await client.post("/api/blueprints/some-id/favorite")
        assert response.status_code == 401


class TestComment:
    """评论功能"""

    async def test_add_comment(self, client: AsyncClient):
        """RED: 添加评论"""
        author_token = await _register_and_login(client, "comm_author", "comm_author@test.com")
        commenter_token = await _register_and_login(client, "commenter", "commenter@test.com")

        headers_a = {"Authorization": f"Bearer {author_token}"}
        create_resp = await client.post("/api/blueprints", json={
            "title": "Commentable MOC",
            "description": "Discuss!",
        }, headers=headers_a)
        bp_id = create_resp.json()["id"]

        headers_c = {"Authorization": f"Bearer {commenter_token}"}
        comment_resp = await client.post(f"/api/blueprints/{bp_id}/comments", json={
            "content": "Nice build!",
        }, headers=headers_c)
        assert comment_resp.status_code == 201
        assert comment_resp.json()["content"] == "Nice build!"
        assert comment_resp.json()["user"]["username"] == "commenter"

    async def test_list_comments(self, client: AsyncClient):
        """RED: 列出图纸评论"""
        token = await _register_and_login(client, "listcomm", "listcomm@test.com")
        headers = {"Authorization": f"Bearer {token}"}

        create_resp = await client.post("/api/blueprints", json={
            "title": "Many Comments",
            "description": "Popular!",
        }, headers=headers)
        bp_id = create_resp.json()["id"]

        for i in range(3):
            await client.post(f"/api/blueprints/{bp_id}/comments", json={
                "content": f"Comment {i}",
            }, headers=headers)

        list_resp = await client.get(f"/api/blueprints/{bp_id}/comments")
        assert list_resp.status_code == 200
        assert len(list_resp.json()) == 3

    async def test_comment_requires_auth(self, client: AsyncClient):
        """RED: 未认证不能评论"""
        response = await client.post("/api/blueprints/some-id/comments", json={
            "content": "Anonymous comment",
        })
        assert response.status_code == 401


# ────────────────────────── Helpers ──────────────────────────

async def _register_and_login(client: AsyncClient, username: str, email: str, password: str = "secret123") -> str:
    """注册并登录，返回 access_token"""
    await client.post("/api/auth/register", json={
        "username": username,
        "email": email,
        "password": password,
    })
    login_resp = await client.post("/api/auth/login", json={
        "email": email,
        "password": password,
    })
    return login_resp.json()["access_token"]
