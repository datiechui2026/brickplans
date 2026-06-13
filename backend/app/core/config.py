from pydantic_settings import BaseSettings
from functools import lru_cache


class Settings(BaseSettings):
    app_name: str = "BrickPlans API"
    debug: bool = True

    # Database
    database_url: str = "postgresql+asyncpg://brickplans:brickplans@db:5432/brickplans"
    database_url_sync: str = "postgresql://brickplans:brickplans@db:5432/brickplans"

    # Redis
    redis_url: str = "redis://redis:6379/0"

    # JWT
    secret_key: str = "change-me-in-production-use-a-long-random-string"
    algorithm: str = "HS256"
    access_token_expire_minutes: int = 30
    refresh_token_expire_days: int = 7

    # Storage: local | tencent_cos
    storage_backend: str = "local"

    # Tencent COS
    tencent_cos_secret_id: str = ""
    tencent_cos_secret_key: str = ""
    tencent_cos_bucket: str = ""
    tencent_cos_region: str = ""
    tencent_cos_public_base_url: str = ""

    # MinIO / S3
    s3_endpoint: str = "http://minio:9000"
    s3_access_key: str = "minioadmin"
    s3_secret_key: str = "minioadmin"
    s3_bucket: str = "brickplans"
    s3_secure: bool = False

    # CORS
    cors_origins: list[str] = ["http://localhost:5173", "http://localhost:3000"]

    model_config = {"env_file": ".env", "env_file_encoding": "utf-8"}


@lru_cache()
def get_settings() -> Settings:
    return Settings()
