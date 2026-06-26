"""
图片上传测试 (RED 阶段 — 测试先于实现)

测试覆盖：
- 上传单张图片 (201)
- 上传多张图片 (201)
- 未认证拒绝 (401)
- 非作者拒绝 (403)
- 非法格式拒绝 (422)
- 蓝图不存在 (404)
- 删除图片
- 设置封面
- 图片重排序
"""
import io

import pytest
from httpx import AsyncClient

# Minimal valid PNG (1x1 white pixel)
MINIMAL_PNG = (
    b'\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR\x00\x00\x00\x01\x00\x00\x00\x01'
    b'\x08\x02\x00\x00\x00\x90wS\xde\x00\x00\x00\x0cIDATx\x9cc\xf8\x0f'
    b'\x00\x00\x01\x01\x00\x05\x18\xd8N\x00\x00\x00\x00IEND\xaeB`\x82'
)


class TestUploadImage:
    """图片上传"""

    async def test_upload_requires_auth(self, client: AsyncClient):
        """RED: 未认证拒绝 401"""
        response = await client.post(
            "/api/blueprints/some-id/images",
            files=[("files", ("test.png", io.BytesIO(MINIMAL_PNG), "image/png"))],
        )
        assert response.status_code == 401

    async def test_upload_nonexistent_blueprint(self, client: AsyncClient):
        """RED: 蓝图不存在返回 404"""
        token = await _register_and_login(client, "up404", "up404@test.com")
        headers = {"Authorization": f"Bearer {token}"}
        response = await client.post(
            "/api/blueprints/00000000-0000-0000-0000-000000000000/images",
            files=[("files", ("test.png", io.BytesIO(MINIMAL_PNG), "image/png"))],
            headers=headers,
        )
        assert response.status_code == 404

    async def test_upload_by_non_author_fails(self, client: AsyncClient):
        """RED: 非作者不能上传 403"""
        # 作者创建蓝图
        author_token = await _register_and_login(client, "img_author", "img_author@test.com")
        headers_author = {"Authorization": f"Bearer {author_token}"}
        create_resp = await client.post("/api/blueprints", json={
            "title": "Author's MOC",
            "description": "Private design",
        }, headers=headers_author)
        bp_id = create_resp.json()["id"]

        # 非作者尝试上传
        other_token = await _register_and_login(client, "intruder_img", "intruder_img@test.com")
        headers_other = {"Authorization": f"Bearer {other_token}"}
        response = await client.post(
            f"/api/blueprints/{bp_id}/images",
            files=[("files", ("hack.png", io.BytesIO(MINIMAL_PNG), "image/png"))],
            headers=headers_other,
        )
        assert response.status_code == 403

    async def test_upload_single_image_succeeds(self, client: AsyncClient):
        """RED: 上传单张图片返回 201 + URL"""
        token = await _register_and_login(client, "uploader1", "uploader1@test.com")
        headers = {"Authorization": f"Bearer {token}"}
        create_resp = await client.post("/api/blueprints", json={
            "title": "Test Blueprint 1",
            "description": "For image upload",
        }, headers=headers)
        bp_id = create_resp.json()["id"]

        response = await client.post(
            f"/api/blueprints/{bp_id}/images",
            files=[("files", ("my-moc.png", io.BytesIO(MINIMAL_PNG), "image/png"))],
            headers=headers,
        )
        assert response.status_code == 201
        data = response.json()
        assert isinstance(data, list)
        assert len(data) == 1
        assert data[0]["url"].startswith("/uploads/")
        assert data[0]["object_key"].startswith("blueprints/")
        assert data[0]["url"] == f"/uploads/{data[0]['object_key']}"
        assert "id" in data[0]
        assert data[0].get("file_type") == "image"

        # 验证图片关联到蓝图
        detail_resp = await client.get(f"/api/blueprints/{bp_id}")
        assert len(detail_resp.json()["images"]) == 1

    async def test_upload_multiple_images_succeeds(self, client: AsyncClient):
        """RED: 上传多张图片全部成功"""
        token = await _register_and_login(client, "uploader2", "uploader2@test.com")
        headers = {"Authorization": f"Bearer {token}"}
        create_resp = await client.post("/api/blueprints", json={
            "title": "Multi Image MOC",
            "description": "Lots of photos",
        }, headers=headers)
        bp_id = create_resp.json()["id"]

        response = await client.post(
            f"/api/blueprints/{bp_id}/images",
            files=[
                ("files", ("photo1.png", io.BytesIO(MINIMAL_PNG), "image/png")),
                ("files", ("photo2.png", io.BytesIO(MINIMAL_PNG), "image/png")),
                ("files", ("photo3.png", io.BytesIO(MINIMAL_PNG), "image/png")),
            ],
            headers=headers,
        )
        assert response.status_code == 201
        data = response.json()
        assert len(data) == 3
        for img in data:
            assert img["url"].startswith("/uploads/")
            assert img["object_key"].startswith("blueprints/")

        # 验证蓝图详情包含 3 张图
        detail_resp = await client.get(f"/api/blueprints/{bp_id}")
        assert len(detail_resp.json()["images"]) == 3

    async def test_append_upload_continues_sort_order(self, client: AsyncClient):
        """追加上传图片时 sort_order 不能从 0 重置。"""
        token = await _register_and_login(client, "append_img", "append_img@test.com")
        headers = {"Authorization": f"Bearer {token}"}
        create_resp = await client.post("/api/blueprints", json={
            "title": "Append Image MOC",
            "description": "Append photos later",
        }, headers=headers)
        bp_id = create_resp.json()["id"]

        first_resp = await client.post(
            f"/api/blueprints/{bp_id}/images",
            files=[("files", ("first.png", io.BytesIO(MINIMAL_PNG), "image/png"))],
            headers=headers,
        )
        assert first_resp.status_code == 201
        assert [img["sort_order"] for img in first_resp.json()] == [0]

        second_resp = await client.post(
            f"/api/blueprints/{bp_id}/images",
            files=[
                ("files", ("second.png", io.BytesIO(MINIMAL_PNG), "image/png")),
                ("files", ("third.png", io.BytesIO(MINIMAL_PNG), "image/png")),
            ],
            headers=headers,
        )
        assert second_resp.status_code == 201
        images = second_resp.json()
        assert [img["sort_order"] for img in images] == [0, 1, 2]

        detail_resp = await client.get(f"/api/blueprints/{bp_id}")
        assert [img["sort_order"] for img in detail_resp.json()["images"]] == [0, 1, 2]

    async def test_invalid_format_rejected(self, client: AsyncClient):
        """RED: 非法格式 (gif) 拒绝 422"""
        token = await _register_and_login(client, "fmtcheck", "fmtcheck@test.com")
        headers = {"Authorization": f"Bearer {token}"}
        create_resp = await client.post("/api/blueprints", json={
            "title": "Format Test MOC",
        }, headers=headers)
        bp_id = create_resp.json()["id"]

        # 上传 .gif（不在允许列表中）
        response = await client.post(
            f"/api/blueprints/{bp_id}/images",
            files=[("files", ("bad.gif", b"GIF89a\x01\x00\x01\x00\x00\x00\x00;", "image/gif"))],
            headers=headers,
        )
        assert response.status_code == 422

        # 上传 .txt（不在允许列表中）
        response = await client.post(
            f"/api/blueprints/{bp_id}/images",
            files=[("files", ("bad.txt", b"hello world this is text", "text/plain"))],
            headers=headers,
        )
        assert response.status_code == 422


class TestDeleteImage:
    """图片删除"""

    async def test_delete_requires_auth(self, client: AsyncClient):
        """RED: 未认证拒绝 401"""
        response = await client.delete("/api/blueprints/some-id/images/some-img")
        assert response.status_code == 401

    async def test_delete_by_non_author_fails(self, client: AsyncClient):
        """RED: 非作者拒绝 403"""
        token_a = await _register_and_login(client, "del_auth", "del_auth@test.com")
        token_b = await _register_and_login(client, "del_intruder", "del_intruder@test.com")

        headers_a = {"Authorization": f"Bearer {token_a}"}
        bp_id = await _create_blueprint(client, headers_a, "Delete Image BP")
        resp = await client.post(f"/api/blueprints/{bp_id}/images",
            files=[("files", ("test.png", io.BytesIO(MINIMAL_PNG), "image/png"))],
            headers=headers_a)
        img_id = resp.json()[0]["id"]

        headers_b = {"Authorization": f"Bearer {token_b}"}
        response = await client.delete(f"/api/blueprints/{bp_id}/images/{img_id}", headers=headers_b)
        assert response.status_code == 403

    async def test_delete_succeeds(self, client: AsyncClient):
        """RED: 作者删除图片成功"""
        token = await _register_and_login(client, "del_ok", "del_ok@test.com")
        headers = {"Authorization": f"Bearer {token}"}
        bp_id = await _create_blueprint(client, headers, "Delete OK BP")

        # Upload 2 images
        resp = await client.post(f"/api/blueprints/{bp_id}/images",
            files=[("files", ("a.png", io.BytesIO(MINIMAL_PNG), "image/png")),
                   ("files", ("b.png", io.BytesIO(MINIMAL_PNG), "image/png"))],
            headers=headers)
        images = resp.json()
        assert len(images) == 2

        # Delete first
        response = await client.delete(f"/api/blueprints/{bp_id}/images/{images[0]['id']}", headers=headers)
        assert response.status_code == 204

        # Verify only 1 remains
        detail = await client.get(f"/api/blueprints/{bp_id}")
        assert len(detail.json()["images"]) == 1
        assert detail.json()["images"][0]["id"] == images[1]["id"]


class TestSetCover:
    """封面设置"""

    async def test_set_cover_requires_auth(self, client: AsyncClient):
        """未认证拒绝 401"""
        response = await client.put("/api/blueprints/some-id/images/some-img/cover")
        assert response.status_code == 401

    async def test_set_cover_by_non_author_fails(self, client: AsyncClient):
        """非作者拒绝 403"""
        token_a = await _register_and_login(client, "cover_auth", "cover_auth@test.com")
        token_b = await _register_and_login(client, "cover_intruder", "cover_intruder@test.com")

        headers_a = {"Authorization": f"Bearer {token_a}"}
        bp_id = await _create_blueprint(client, headers_a, "Cover BP")
        resp = await client.post(f"/api/blueprints/{bp_id}/images",
            files=[("files", ("test.png", io.BytesIO(MINIMAL_PNG), "image/png"))],
            headers=headers_a)
        img_id = resp.json()[0]["id"]

        headers_b = {"Authorization": f"Bearer {token_b}"}
        response = await client.put(f"/api/blueprints/{bp_id}/images/{img_id}/cover", headers=headers_b)
        assert response.status_code == 403

    async def test_set_cover_succeeds(self, client: AsyncClient):
        """设置封面成功"""
        token = await _register_and_login(client, "cover_ok", "cover_ok@test.com")
        headers = {"Authorization": f"Bearer {token}"}
        bp_id = await _create_blueprint(client, headers, "Cover OK BP")

        # Upload 3 images
        resp = await client.post(f"/api/blueprints/{bp_id}/images",
            files=[("files", ("a.png", io.BytesIO(MINIMAL_PNG), "image/png")),
                   ("files", ("b.png", io.BytesIO(MINIMAL_PNG), "image/png")),
                   ("files", ("c.png", io.BytesIO(MINIMAL_PNG), "image/png"))],
            headers=headers)
        images = resp.json()
        assert len(images) == 3

        # Set the second image as cover
        response = await client.put(f"/api/blueprints/{bp_id}/images/{images[1]['id']}/cover", headers=headers)
        assert response.status_code == 200
        assert response.json() == {"message": "ok"}

        # Verify only the second image is cover
        detail = await client.get(f"/api/blueprints/{bp_id}")
        detail_images = detail.json()["images"]
        assert detail_images[1]["is_cover"] is True
        assert detail_images[0]["is_cover"] is False
        assert detail_images[2]["is_cover"] is False

    async def test_set_cover_nonexistent_image(self, client: AsyncClient):
        """设置不存在的图片为封面返回 404"""
        token = await _register_and_login(client, "cover_404", "cover_404@test.com")
        headers = {"Authorization": f"Bearer {token}"}
        bp_id = await _create_blueprint(client, headers, "Cover 404 BP")

        response = await client.put(
            f"/api/blueprints/{bp_id}/images/00000000-0000-0000-0000-000000000000/cover",
            headers=headers,
        )
        assert response.status_code == 404


class TestReorderImages:
    """图片重排序"""

    async def test_reorder_requires_auth(self, client: AsyncClient):
        """未认证拒绝 401"""
        response = await client.put("/api/blueprints/some-id/images/reorder",
            json={"images": []})
        assert response.status_code == 401

    async def test_reorder_by_non_author_fails(self, client: AsyncClient):
        """非作者拒绝 403"""
        token_a = await _register_and_login(client, "reorder_auth", "reorder_auth@test.com")
        token_b = await _register_and_login(client, "reorder_intruder", "reorder_intruder@test.com")

        headers_a = {"Authorization": f"Bearer {token_a}"}
        bp_id = await _create_blueprint(client, headers_a, "Reorder BP")

        headers_b = {"Authorization": f"Bearer {token_b}"}
        response = await client.put(f"/api/blueprints/{bp_id}/images/reorder",
            json={"images": []}, headers=headers_b)
        assert response.status_code == 403

    async def test_reorder_succeeds(self, client: AsyncClient):
        """重新排序成功"""
        token = await _register_and_login(client, "reorder_ok", "reorder_ok@test.com")
        headers = {"Authorization": f"Bearer {token}"}
        bp_id = await _create_blueprint(client, headers, "Reorder OK BP")

        # Upload 3 images
        resp = await client.post(f"/api/blueprints/{bp_id}/images",
            files=[("files", ("a.png", io.BytesIO(MINIMAL_PNG), "image/png")),
                   ("files", ("b.png", io.BytesIO(MINIMAL_PNG), "image/png")),
                   ("files", ("c.png", io.BytesIO(MINIMAL_PNG), "image/png"))],
            headers=headers)
        images = resp.json()
        ids = [img["id"] for img in images]

        # Reverse the order
        reversed_ids = list(reversed(ids))
        payload = {
            "images": [
                {"id": reversed_ids[0], "sort_order": 0},
                {"id": reversed_ids[1], "sort_order": 1},
                {"id": reversed_ids[2], "sort_order": 2},
            ]
        }
        response = await client.put(
            f"/api/blueprints/{bp_id}/images/reorder",
            json=payload,
            headers=headers,
        )
        assert response.status_code == 200
        assert response.json() == {"message": "ok"}

        # Verify order is reversed
        detail = await client.get(f"/api/blueprints/{bp_id}")
        detail_images = detail.json()["images"]
        assert detail_images[0]["id"] == reversed_ids[0]
        assert detail_images[1]["id"] == reversed_ids[1]
        assert detail_images[2]["id"] == reversed_ids[2]


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


async def _create_blueprint(client: AsyncClient, headers: dict, title: str) -> str:
    resp = await client.post("/api/blueprints", json={
        "title": title, "description": "Test",
    }, headers=headers)
    return resp.json()["id"]


# io module used in test functions
import io
