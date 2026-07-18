package handler

import (
	"log"

	"gorm.io/gorm"

	"brickplans/internal/config"
	"brickplans/internal/db"
	"brickplans/internal/storage"
)

// deleteBlueprintImageFiles best-effort 删除某图纸所有图片/PDF 的存储文件。
// 必须在 DB 级联删除之前调用：GORM OnDelete:CASCADE 只删 BlueprintImage 数据库
// 行，不会删 LocalStorage/COS 上的物理文件，否则会留下孤儿文件。单个文件删除
// 失败仅记日志，不阻塞业务删除（DB 行仍会被清理）。
func deleteBlueprintImageFiles(cfg *config.Config, gdb *gorm.DB, blueprintID string) {
	st, err := storage.Get(cfg)
	if err != nil {
		log.Printf("[cleanup] storage unavailable for blueprint %s: %v", blueprintID, err)
		return
	}
	var imgs []db.BlueprintImage
	if err := gdb.Where("blueprint_id = ?", blueprintID).Find(&imgs).Error; err != nil {
		log.Printf("[cleanup] load images for blueprint %s: %v", blueprintID, err)
		return
	}
	for _, im := range imgs {
		key := im.ObjectKey
		if key == "" {
			key = im.URL
		}
		if err := st.Delete(key); err != nil {
			log.Printf("[cleanup] delete storage object %s: %v", key, err)
		}
	}
}
