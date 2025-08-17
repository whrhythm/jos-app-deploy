package model

import (
	"time"
)

// JosApp 对应数据库表 jos_app
type JosApp struct {
	ID                    uint64    `gorm:"column:id;primaryKey;autoIncrement"`
	AppID                 uint64    `gorm:"column:app_id;not null"`                           // 应用ID
	WorkspaceID           uint64    `gorm:"column:workspace_id;not null;index"`               // 工作空间ID（带索引）
	ProjectID             uint64    `gorm:"column:project_id;not null"`                       // 项目ID
	ProjectName           string    `gorm:"column:project_name;type:varchar(255)"`            // 项目名称
	AppName               string    `gorm:"column:app_name;type:varchar(255);not null"`       // 应用名称
	EnvID                 uint64    `gorm:"column:env_id"`                                    // 环境ID
	AppIcon               string    `gorm:"column:app_icon;type:varchar(500)"`                // 应用图标URL
	PublishAddressInside  string    `gorm:"column:publish_address_inside;type:varchar(255)"`  // 内部发布地址
	PublishAddressOutside string    `gorm:"column:publish_address_outside;type:varchar(255)"` // 外部发布地址
	Status                string    `gorm:"column:status;type:varchar(255)"`                  // 状态
	ClientID              uint64    `gorm:"column:client_id"`                                 // 客户端ID
	AppClientID           uint64    `gorm:"column:app_client_id"`                             // 应用客户端ID
	CreateDate            time.Time `gorm:"column:create_date;default:CURRENT_TIMESTAMP"`     // 创建时间（自动设置）
	ModifyDate            time.Time `gorm:"column:modify_date;autoUpdateTime"`                // 修改时间（自动更新）
}

// TableName 指定表名
func (JosApp) TableName() string {
	return "jos_app"
}
