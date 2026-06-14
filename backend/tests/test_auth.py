"""Tests for auth API: register, login, token refresh."""

import pytest

from app.services.storage import StoredObject


class TestRegister:
    async def test_register_creates_user_and_returns_tokens(self, client):
        """RED: 注册新用户应返回 201 + access/refresh token"""
        response = await client.post("/api/auth/register", json={
            "username": "testuser",
            "email": "test@example.com",
            "password": "secret123",
        })

        assert response.status_code == 201
        data = response.json()
        assert "access_token" in data
        assert "refresh_token" in data
        assert data["token_type"] == "bearer"

    async def test_register_duplicate_email_fails(self, client):
        """RED: 重复邮箱注册应返回 409"""
        payload = {"username": "user1", "email": "dup@example.com", "password": "secret123"}
        await client.post("/api/auth/register", json=payload)

        response = await client.post("/api/auth/register", json={
            "username": "user2",
            "email": "dup@example.com",
            "password": "secret456",
        })

        assert response.status_code == 409

    async def test_register_short_password_fails(self, client):
        """RED: 密码少于6位应返回 422"""
        response = await client.post("/api/auth/register", json={
            "username": "user",
            "email": "user@example.com",
            "password": "123",
        })

        assert response.status_code == 422


class TestLogin:
    async def test_login_with_correct_credentials_returns_tokens(self, client):
        """RED: 正确凭据登录应返回 token"""
        # First register
        await client.post("/api/auth/register", json={
            "username": "loginuser",
            "email": "login@example.com",
            "password": "secret123",
        })

        # Then login
        response = await client.post("/api/auth/login", json={
            "email": "login@example.com",
            "password": "secret123",
        })

        assert response.status_code == 200
        data = response.json()
        assert "access_token" in data
        assert "refresh_token" in data

    async def test_login_wrong_password_fails(self, client):
        """RED: 错误密码登录应返回 401"""
        await client.post("/api/auth/register", json={
            "username": "pwuser",
            "email": "pw@example.com",
            "password": "secret123",
        })

        response = await client.post("/api/auth/login", json={
            "email": "pw@example.com",
            "password": "wrongpassword",
        })

        assert response.status_code == 401


class TestGetMe:
    async def test_get_me_requires_auth(self, client):
        """无 token 访问 /me 应返回 401"""
        response = await client.get("/api/auth/me")
        assert response.status_code == 401

    async def test_get_me_returns_user_info(self, client):
        """有效 token 访问 /me 应返回当前用户信息"""
        # Register and get token
        resp = await client.post("/api/auth/register", json={
            "username": "meuser",
            "email": "me@example.com",
            "password": "secret123",
        })
        token = resp.json()["access_token"]

        response = await client.get("/api/auth/me", headers={
            "Authorization": f"Bearer {token}",
        })

        assert response.status_code == 200
        data = response.json()
        assert data["username"] == "meuser"
        assert data["email"] == "me@example.com"
        assert "id" in data
        assert "created_at" in data


class TestAvatarUpload:
    async def test_upload_avatar_uses_storage_backend(self, client, monkeypatch):
        class FakeStorage:
            def __init__(self):
                self.calls = []

            async def upload(self, file_data, filename, content_type, prefix="blueprints"):
                self.calls.append((file_data, filename, content_type, prefix))
                return StoredObject(
                    url="https://cos.example.com/avatars/avatar.png",
                    object_key="avatars/avatar.png",
                )

            async def delete(self, url_or_key):
                return None

        fake_storage = FakeStorage()
        monkeypatch.setattr("app.api.auth.get_storage", lambda: fake_storage)

        reg = await client.post("/api/auth/register", json={
            "username": "avataruser",
            "email": "avatar@example.com",
            "password": "secret123",
        })
        token = reg.json()["access_token"]

        resp = await client.post(
            "/api/auth/avatar",
            files={"file": ("avatar.png", b"fake-png", "image/png")},
            headers={"Authorization": f"Bearer {token}"},
        )

        assert resp.status_code == 200
        data = resp.json()
        assert data["avatar_url"] == "https://cos.example.com/avatars/avatar.png"
        assert data["object_key"] == "avatars/avatar.png"
        assert data["user"]["avatar_url"] == data["avatar_url"]
        assert fake_storage.calls[0][3] == "avatars"
