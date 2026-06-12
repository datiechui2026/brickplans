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
from pathlib import Path
from typing import Optional

UPLOAD_DIR = Path(__file__).resolve().parent.parent.parent / "uploads"


class BaseStorage(ABC):
    """存储后端抽象基类"""

    @abstractmethod
    async def upload(self, file_data: bytes, filename: str, content_type: str) -> str:
        """上传文件，返回可访问的 URL"""
        ...

    @abstractmethod
    async def delete(self, url_or_key: str) -> None:
        """删除文件"""
        ...

    @staticmethod
    def _make_object_key(filename: str) -> str:
        """生成唯一 object key: {uuid}_{safe_name}"""
        ext = Path(filename).suffix or ""
        return f"{uuid.uuid4().hex[:12]}{ext}"


class LocalStorage(BaseStorage):
    """本地文件系统存储"""

    def __init__(self, upload_dir: Optional[Path] = None):
        self.upload_dir = upload_dir or UPLOAD_DIR
        self.upload_dir.mkdir(parents=True, exist_ok=True)

    async def upload(self, file_data: bytes, filename: str, content_type: str = "application/octet-stream") -> str:
        object_key = self._make_object_key(filename)
        file_path = self.upload_dir / object_key
        file_path.write_bytes(file_data)
        return f"/uploads/{object_key}"

    async def delete(self, url_or_key: str) -> None:
        # url_or_key is like "/uploads/abc123_file.png"
        object_key = url_or_key.rsplit("/", 1)[-1] if "/" in url_or_key else url_or_key
        file_path = self.upload_dir / object_key
        if file_path.exists():
            file_path.unlink()


class MinIOStorage(BaseStorage):
    """MinIO 对象存储 — 需要 pip install minio"""

    def __init__(self, endpoint: str, access_key: str, secret_key: str, bucket: str):
        self.endpoint = endpoint
        self.access_key = access_key
        self.secret_key = secret_key
        self.bucket = bucket
        self._client = None

    async def upload(self, file_data: bytes, filename: str, content_type: str = "application/octet-stream") -> str:
        # TODO: Install minio package and implement
        # client = self._get_client()
        # object_key = f"blueprints/{self._make_object_key(filename)}"
        # client.put_object(self.bucket, object_key, io.BytesIO(file_data), len(file_data), content_type=content_type)
        # return f"{self.endpoint}/{self.bucket}/{object_key}"
        raise NotImplementedError("MinIO storage requires 'pip install minio'. Use LocalStorage for now.")

    async def delete(self, url_or_key: str) -> None:
        raise NotImplementedError("MinIO storage requires 'pip install minio'.")


_storage_instance: Optional[BaseStorage] = None


def get_storage() -> BaseStorage:
    """获取存储实例（单例）。默认使用本地存储。"""
    global _storage_instance
    if _storage_instance is None:
        _storage_instance = LocalStorage()
    return _storage_instance
