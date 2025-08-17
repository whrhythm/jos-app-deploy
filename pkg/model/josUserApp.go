package model

import (
	"time"
)

// JosUserApp 用户应用关联表
// 新增 AppClientID 字段
// ALTER TABLE jos_user_app ADD COLUMN app_client_id BIGINT COMMENT '客户端ID';
type JosUserApp struct {
	ID             uint64    `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	UserID         uint64    `gorm:"column:user_id;not null;index" json:"userId"`                                            // 用户ID（带索引）
	AppID          uint64    `gorm:"column:app_id;not null" json:"appId"`                                                    // 应用ID
	AppName        string    `gorm:"column:app_name;type:varchar(100);not null" json:"appName"`                              // 应用名称
	ClientID       uint64    `gorm:"column:client_id" json:"clientId"`                                                       // 客户端ID
	CreateDate     time.Time `gorm:"column:create_date;not null;default:CURRENT_TIMESTAMP" json:"createDate"`                // 创建时间
	CreateUserID   string    `gorm:"column:create_user_id;type:varchar(50)" json:"createUserId"`                             // 创建人ID
	CreateUserName string    `gorm:"column:create_user_name;type:varchar(50)" json:"createUserName"`                         // 创建人姓名
	ModifyDate     time.Time `gorm:"column:modify_date;not null;default:CURRENT_TIMESTAMP;autoUpdateTime" json:"modifyDate"` // 修改时间（自动更新）
	ModifyUserID   string    `gorm:"column:modify_user_id;type:varchar(50)" json:"modifyUserId"`                             // 修改人ID
	ModifyUserName string    `gorm:"column:modify_user_name;type:varchar(50)" json:"modifyUserName"`                         // 修改人姓名
}

// TableName 指定表名
func (JosUserApp) TableName() string {
	return "jos_user_app"
}
