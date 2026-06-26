"""
存储抽象层 — 支持本地文件系统 / MinIO

使用方式:
    from app.services.storage import get_storage
    storage = get_storage()
    url = await storage.upload(file_data, filename, content_type)
    await storage.delete(object_key)
"""
import uuid
from abc import ABC, abstractmethod
from dataclasses import dataclass
from io import BytesIO
from pathlib import Path
from typing import Optional

from app.core.config import get_settings

UPLOAD_DIR = Path(__file__).resolve().parent.parent.parent / "uploads"


@dataclass(frozen=True)
class StoredObject:
    """Uploaded object metadata persisted by API callers."""

    url: str
    object_key: str


class BaseStorage(ABC):
    """存储后端抽象基类"""

    @abstractmethod
    async def upload(self, file_data: bytes, filename: str, content_type: str, prefix: str = "blueprints") -> StoredObject:
        """上传文件，返回可访问 URL 和存储 object key。"""
        ...

    @abstractmethod
    async def delete(self, url_or_key: str) -> None:
        """删除文件"""
        ...

    @staticmethod
    def _make_object_key(filename: str, prefix: str = "blueprints") -> str:
        """生成唯一 object key: {prefix}/{uuid}{ext}"""
        ext = Path(filename).suffix or ""
        clean_prefix = prefix.strip("/") or "blueprints"
        return f"{clean_prefix}/{uuid.uuid4().hex[:12]}{ext}"


class LocalStorage(BaseStorage):
    """本地文件系统存储"""

    def __init__(self, upload_dir: Optional[Path] = None):
        self.upload_dir = upload_dir or UPLOAD_DIR
        self.upload_dir.mkdir(parents=True, exist_ok=True)

    async def upload(
        self,
        file_data: bytes,
        filename: str,
        content_type: str = "application/octet-stream",
        prefix: str = "blueprints",
    ) -> StoredObject:
        object_key = self._make_object_key(filename, prefix)
        file_path = self.upload_dir / object_key
        file_path.parent.mkdir(parents=True, exist_ok=True)
        file_path.write_bytes(file_data)
        return StoredObject(url=f"/uploads/{object_key}", object_key=object_key)

    async def delete(self, url_or_key: str) -> None:
        object_key = _object_key_from_url_or_key(url_or_key)
        file_path = self.upload_dir / object_key
        if file_path.exists():
            file_path.unlink()


class TencentCOSStorage(BaseStorage):
    """Tencent COS object storage."""

    def __init__(self):
        try:
            from qcloud_cos import CosConfig, CosS3Client
        except ImportError as exc:
            raise RuntimeError("Tencent COS storage requires 'cos-python-sdk-v5'. Run pip install -r requirements.txt.") from exc

        settings = get_settings()
        self.bucket = settings.tencent_cos_bucket
        self.region = settings.tencent_cos_region
        self.public_base_url = (
            settings.tencent_cos_public_base_url.rstrip("/")
            or f"https://{self.bucket}.cos.{self.region}.myqcloud.com"
        )
        config = CosConfig(
            Region=self.region,
            SecretId=settings.tencent_cos_secret_id,
            SecretKey=settings.tencent_cos_secret_key,
            Scheme="https",
        )
        self.client = CosS3Client(config)

    async def upload(
        self,
        file_data: bytes,
        filename: str,
        content_type: str = "application/octet-stream",
        prefix: str = "blueprints",
    ) -> StoredObject:
        object_key = self._make_object_key(filename, prefix)
        kwargs = {}
        if content_type == "application/pdf":
            kwargs["ContentDisposition"] = "inline"
        self.client.put_object(
            Bucket=self.bucket,
            Body=BytesIO(file_data),
            Key=object_key,
            ContentType=content_type,
            ACL="public-read",
            **kwargs,
        )
        return StoredObject(url=f"{self.public_base_url}/{object_key}", object_key=object_key)

    async def delete(self, url_or_key: str) -> None:
        object_key = _object_key_from_url_or_key(url_or_key, self.public_base_url)
        self.client.delete_object(Bucket=self.bucket, Key=object_key)


def _object_key_from_url_or_key(url_or_key: str, public_base_url: str | None = None) -> str:
    value = url_or_key.strip()
    if public_base_url and value.startswith(public_base_url.rstrip("/") + "/"):
        return value[len(public_base_url.rstrip("/") + "/"):]
    if value.startswith("/uploads/"):
        return value[len("/uploads/"):]
    return value.lstrip("/")


_storage_instance: Optional[BaseStorage] = None


def get_storage() -> BaseStorage:
    """获取存储实例（单例）。默认使用本地存储。"""
    global _storage_instance
    if _storage_instance is None:
        settings = get_settings()
        if settings.storage_backend == "tencent_cos":
            _storage_instance = TencentCOSStorage()
        else:
            _storage_instance = LocalStorage()
    return _storage_instance
