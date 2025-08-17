package model

import "time"

// JosApp 对应数据库表 jos_app
type ProxyUserApp struct {
	ID         int64     `gorm:"column:id;primaryKey" json:"id"`
	UserName   string    `gorm:"column:user_name;type:varchar(25)" json:"userName"` // 账号
	Name       string    `gorm:"column:name;type:varchar(20);index" json:"name"`
	Gender     int       `gorm:"column:gender" json:"gender"`
	Mobile     string    `gorm:"column:mobile;type:varchar(255)" json:"mobile"`
	Email      string    `gorm:"column:email;type:varchar(60)" json:"email"`
	AppID      uint64    `gorm:"column:app_id;not null"`                                 // 应用ID
	AppAddress string    `gorm:"column:app_address;type:varchar(255)" json:"appAddress"` // 应用地址
	AppUserID  uint64    `gorm:"column:app_user_id"`                                     // 应用客户ID
	CreateDate time.Time `gorm:"column:create_date;default:CURRENT_TIMESTAMP"`           // 创建时间（自动设置）
	ModifyDate time.Time `gorm:"column:modify_date;autoUpdateTime"`                      // 修改时间（自动更新）
}

func (ProxyUserApp) TableName() string {
	return "proxy_user_app"
}
