"""Tests for auth API: register, login, token refresh."""

import pytest


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
