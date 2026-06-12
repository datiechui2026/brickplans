"""
SEO endpoints: sitemap.xml
"""
from datetime import datetime, timezone

from fastapi import APIRouter, Depends
from fastapi.responses import Response
from sqlalchemy import select
from sqlalchemy.ext.asyncio import AsyncSession

from app.core.database import get_db
from app.models import Blueprint

router = APIRouter(tags=["seo"])

SITEMAP_TEMPLATE = """<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
{urls}
</urlset>"""

URL_TEMPLATE = """  <url>
    <loc>{loc}</loc>
    <lastmod>{lastmod}</lastmod>
  </url>"""


@router.get("/sitemap.xml", response_class=Response)
async def sitemap(db: AsyncSession = Depends(get_db)):
    """Return XML sitemap of all published blueprints."""
    result = await db.execute(
        select(Blueprint.id, Blueprint.slug, Blueprint.updated_at)
        .where(Blueprint.is_published == True)
        .order_by(Blueprint.updated_at.desc())
    )
    blueprints = result.all()

    base_url = "https://brickplans.com"

    urls = []
    for bp_id, bp_slug, bp_updated in blueprints:
        updated_at = bp_updated or datetime.min.replace(tzinfo=timezone.utc)
        lastmod = updated_at.strftime("%Y-%m-%d")
        # Use slug in URL for SEO-friendly URLs
        urls.append(URL_TEMPLATE.format(
            loc=f"{base_url}/#/detail?id={bp_id}",
            lastmod=lastmod,
        ))

    xml_content = SITEMAP_TEMPLATE.format(urls="\n".join(urls))
    return Response(content=xml_content, media_type="application/xml")
