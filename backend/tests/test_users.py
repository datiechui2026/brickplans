"""Tests for user profile API."""
import pytest


class TestGetUserProfile:
    async def test_get_nonexistent_user_returns_404(self, client):
        resp = await client.get("/api/users/nonexistentuser")
        assert resp.status_code == 404

    async def test_get_user_profile_returns_info_and_stats(self, client):
        # Register a user
        reg = await client.post("/api/auth/register", json={
            "username": "profileuser",
            "email": "profile@example.com",
            "password": "secret123",
        })
        token = reg.json()["access_token"]

        # Create a blueprint
        await client.post("/api/blueprints", json={
            "title": "My Cool Build",
            "description": "Test",
            "category": "建筑",
            "difficulty": 3,
            "piece_count": 500,
            "is_published": True,
        }, headers={"Authorization": f"Bearer {token}"})

        # Get profile
        resp = await client.get("/api/users/profileuser")
        assert resp.status_code == 200
        data = resp.json()
        assert data["username"] == "profileuser"
        assert data["email"] == "profile@example.com"
        assert data["blueprint_count"] == 1
        assert data["favorite_count"] == 0
        assert "id" in data
        assert "created_at" in data


class TestUserBlueprints:
    async def test_user_blueprints_returns_paginated(self, client):
        # Register and create 2 blueprints
        reg = await client.post("/api/auth/register", json={
            "username": "bpuser",
            "email": "bpuser@example.com",
            "password": "secret123",
        })
        token = reg.json()["access_token"]

        for i in range(2):
            await client.post("/api/blueprints", json={
                "title": f"Build {i}",
                "description": "Test",
                "category": "建筑",
                "is_published": True,
            }, headers={"Authorization": f"Bearer {token}"})

        resp = await client.get("/api/users/bpuser/blueprints?size=10")
        assert resp.status_code == 200
        data = resp.json()
        assert data["total"] == 2
        assert len(data["items"]) == 2

    async def test_user_blueprints_nonexistent_user(self, client):
        resp = await client.get("/api/users/nobody/blueprints")
        assert resp.status_code == 404


class TestUserFavorites:
    async def test_user_favorites_returns_favorited(self, client):
        # Register two users
        reg1 = await client.post("/api/auth/register", json={
            "username": "author1",
            "email": "author1@example.com",
            "password": "secret123",
        })
        token1 = reg1.json()["access_token"]

        reg2 = await client.post("/api/auth/register", json={
            "username": "fan1",
            "email": "fan1@example.com",
            "password": "secret123",
        })
        token2 = reg2.json()["access_token"]

        # Author creates a blueprint
        bp = await client.post("/api/blueprints", json={
            "title": "FavTest",
            "description": "Test",
            "category": "机甲",
            "is_published": True,
        }, headers={"Authorization": f"Bearer {token1}"})
        bp_id = bp.json()["id"]

        # Fan favorites it
        await client.post(f"/api/blueprints/{bp_id}/favorite",
                          headers={"Authorization": f"Bearer {token2}"})

        # Check fan's favorites
        resp = await client.get("/api/users/fan1/favorites")
        assert resp.status_code == 200
        data = resp.json()
        assert data["total"] == 1
        assert len(data["items"]) == 1
        assert data["items"][0]["title"] == "FavTest"

    async def test_user_favorites_nonexistent_user(self, client):
        resp = await client.get("/api/users/nobody/favorites")
        assert resp.status_code == 404