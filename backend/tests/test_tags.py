"""
标签 API 测试 (RED 阶段)

测试覆盖：
- 获取全部标签 (GET /api/tags)
- 给蓝图打标签 (POST /api/blueprints/{id}/tags) — 批量、自动创建、幂等
- 获取蓝图标签 (GET /api/blueprints/{id}/tags)
- 移除标签 (DELETE /api/blueprints/{id}/tags/{tag_id})
- 权限校验 (401/403)
"""
import pytest
from httpx import AsyncClient


class TestListAllTags:
    """获取全部标签"""

    async def test_list_tags_empty(self, client: AsyncClient):
        """RED: 无标签时返回空列表"""
        response = await client.get("/api/tags")
        assert response.status_code == 200
        assert response.json() == []

    async def test_list_tags_after_creation(self, client: AsyncClient):
        """RED: 打标签后全部标签列表包含新标签"""
        token = await _register_and_login(client, "taglister", "taglister@test.com")
        headers = {"Authorization": f"Bearer {token}"}

        # Create blueprint
        bp_id = await _create_blueprint(client, headers, "TagTest BP")

        # Bind tags (creates new tags)
        await client.post(
            f"/api/blueprints/{bp_id}/tags",
            json={"tags": ["机甲", "科幻", "建筑"]},
            headers=headers,
        )

        # List all tags
        response = await client.get("/api/tags")
        assert response.status_code == 200
        data = response.json()
        assert len(data) == 3
        names = {t["name"] for t in data}
        assert names == {"机甲", "科幻", "建筑"}


class TestBindTags:
    """给蓝图打标签"""

    async def test_bind_tags_requires_auth(self, client: AsyncClient):
        """RED: 未认证拒绝 401"""
        response = await client.post(
            "/api/blueprints/some-id/tags",
            json={"tags": ["机甲"]},
        )
        assert response.status_code == 401

    async def test_bind_tags_by_non_author_fails(self, client: AsyncClient):
        """RED: 非作者拒绝 403"""
        token_a = await _register_and_login(client, "tag_author", "tag_author@test.com")
        token_b = await _register_and_login(client, "tag_intruder", "tag_intruder@test.com")

        headers_a = {"Authorization": f"Bearer {token_a}"}
        bp_id = await _create_blueprint(client, headers_a, "Author BP")

        headers_b = {"Authorization": f"Bearer {token_b}"}
        response = await client.post(
            f"/api/blueprints/{bp_id}/tags",
            json={"tags": ["捣乱"]},
            headers=headers_b,
        )
        assert response.status_code == 403

    async def test_bind_tags_creates_new(self, client: AsyncClient):
        """RED: 新标签自动创建并关联"""
        token = await _register_and_login(client, "tagbinder", "tagbinder@test.com")
        headers = {"Authorization": f"Bearer {token}"}
        bp_id = await _create_blueprint(client, headers, "New Tags BP")

        response = await client.post(
            f"/api/blueprints/{bp_id}/tags",
            json={"tags": ["机甲", "科幻"]},
            headers=headers,
        )
        assert response.status_code == 201
        data = response.json()
        assert len(data) == 2
        tag_names = {t["name"] for t in data}
        assert tag_names == {"机甲", "科幻"}
        for t in data:
            assert "id" in t

    async def test_bind_tags_idempotent(self, client: AsyncClient):
        """RED: 重复打相同标签不报错不重复"""
        token = await _register_and_login(client, "idempoter", "idempoter@test.com")
        headers = {"Authorization": f"Bearer {token}"}
        bp_id = await _create_blueprint(client, headers, "Idempotent BP")

        # First bind
        await client.post(
            f"/api/blueprints/{bp_id}/tags",
            json={"tags": ["机甲", "科幻"]},
            headers=headers,
        )

        # Second bind with partial overlap
        response = await client.post(
            f"/api/blueprints/{bp_id}/tags",
            json={"tags": ["机甲", "建筑"]},
            headers=headers,
        )
        assert response.status_code == 201
        data = response.json()
        assert len(data) == 3  # 机甲(already) + 科幻(existing) + 建筑(new) = 3 unique
        tag_names = {t["name"] for t in data}
        assert tag_names == {"机甲", "科幻", "建筑"}

    async def test_bind_tags_blueprint_not_found(self, client: AsyncClient):
        """RED: 蓝图不存在 404"""
        token = await _register_and_login(client, "tag404", "tag404@test.com")
        headers = {"Authorization": f"Bearer {token}"}
        response = await client.post(
            "/api/blueprints/00000000-0000-0000-0000-000000000000/tags",
            json={"tags": ["机甲"]},
            headers=headers,
        )
        assert response.status_code == 404


class TestGetBlueprintTags:
    """获取蓝图标签"""

    async def test_get_blueprint_tags(self, client: AsyncClient):
        """RED: 获取蓝图标签列表"""
        token = await _register_and_login(client, "tag_getter", "tag_getter@test.com")
        headers = {"Authorization": f"Bearer {token}"}
        bp_id = await _create_blueprint(client, headers, "Get Tags BP")

        await client.post(
            f"/api/blueprints/{bp_id}/tags",
            json={"tags": ["机甲", "科幻"]},
            headers=headers,
        )

        response = await client.get(f"/api/blueprints/{bp_id}/tags")
        assert response.status_code == 200
        data = response.json()
        assert len(data) == 2
        assert {t["name"] for t in data} == {"机甲", "科幻"}

    async def test_get_tags_empty_blueprint(self, client: AsyncClient):
        """RED: 无标签蓝图返回空列表"""
        token = await _register_and_login(client, "tag_empty", "tag_empty@test.com")
        headers = {"Authorization": f"Bearer {token}"}
        bp_id = await _create_blueprint(client, headers, "No Tags BP")

        response = await client.get(f"/api/blueprints/{bp_id}/tags")
        assert response.status_code == 200
        assert response.json() == []


class TestRemoveTag:
    """移除标签"""

    async def test_remove_tag_requires_auth(self, client: AsyncClient):
        """RED: 未认证拒绝 401"""
        response = await client.delete("/api/blueprints/some-id/tags/some-tag")
        assert response.status_code == 401

    async def test_remove_tag_by_non_author_fails(self, client: AsyncClient):
        """RED: 非作者拒绝 403"""
        token_a = await _register_and_login(client, "rm_author", "rm_author@test.com")
        token_b = await _register_and_login(client, "rm_intruder", "rm_intruder@test.com")

        headers_a = {"Authorization": f"Bearer {token_a}"}
        bp_id = await _create_blueprint(client, headers_a, "Remove Tag BP")

        # Bind a tag
        resp = await client.post(
            f"/api/blueprints/{bp_id}/tags",
            json={"tags": ["机甲"]},
            headers=headers_a,
        )
        tag_id = resp.json()[0]["id"]

        headers_b = {"Authorization": f"Bearer {token_b}"}
        response = await client.delete(
            f"/api/blueprints/{bp_id}/tags/{tag_id}",
            headers=headers_b,
        )
        assert response.status_code == 403

    async def test_remove_tag_succeeds(self, client: AsyncClient):
        """RED: 移除标签关联，Tag 本身保留"""
        token = await _register_and_login(client, "rm_ok", "rm_ok@test.com")
        headers = {"Authorization": f"Bearer {token}"}
        bp_id = await _create_blueprint(client, headers, "Remove OK BP")

        resp = await client.post(
            f"/api/blueprints/{bp_id}/tags",
            json={"tags": ["机甲", "科幻"]},
            headers=headers,
        )
        tags = resp.json()
        tag_to_remove = next(t for t in tags if t["name"] == "机甲")

        # Remove
        response = await client.delete(
            f"/api/blueprints/{bp_id}/tags/{tag_to_remove['id']}",
            headers=headers,
        )
        assert response.status_code == 204

        # Verify blueprint tags updated
        get_resp = await client.get(f"/api/blueprints/{bp_id}/tags")
        remaining = get_resp.json()
        assert len(remaining) == 1
        assert remaining[0]["name"] == "科幻"

        # Verify Tag itself still exists in global list
        all_tags = await client.get("/api/tags")
        assert len(all_tags.json()) == 2  # 机甲 + 科幻 both still exist


# ────────────────────────── Helpers ──────────────────────────

async def _register_and_login(client: AsyncClient, username: str, email: str, password: str = "secret123") -> str:
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


async def _create_blueprint(client: AsyncClient, headers: dict, title: str) -> str:
    resp = await client.post("/api/blueprints", json={
        "title": title,
        "description": "Test blueprint",
    }, headers=headers)
    return resp.json()["id"]
