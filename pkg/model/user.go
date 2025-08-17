package model

import "time"

type XjrUser struct {
	ID           int64     `gorm:"column:id;primaryKey" json:"id"`
	UserName     string    `gorm:"column:user_name;type:varchar(25)" json:"userName"`
	Name         string    `gorm:"column:name;type:varchar(20);index" json:"name"`
	Code         string    `gorm:"column:code;type:varchar(20)" json:"code"`
	NickName     string    `gorm:"column:nick_name;type:varchar(50)" json:"nickName"`
	Password     string    `gorm:"column:password;type:varchar(50)" json:"-"`
	Gender       int       `gorm:"column:gender" json:"gender"`
	Mobile       string    `gorm:"column:mobile;type:varchar(255)" json:"mobile"`
	Avatar       string    `gorm:"column:avatar;type:varchar(2000)" json:"avatar"`
	Email        string    `gorm:"column:email;type:varchar(60)" json:"email"`
	Address      string    `gorm:"column:address;type:varchar(200)" json:"address"`
	Longitude    float64   `gorm:"column:longitude" json:"longitude"`
	Latitude     float64   `gorm:"column:latitude" json:"latitude"`
	SortCode     int       `gorm:"column:sort_code" json:"sortCode"`
	Remark       string    `gorm:"column:remark;type:varchar(255)" json:"remark"`
	LoginTimes   int       `gorm:"column:login_times;default:0" json:"loginTimes"`
	CreateUserID int64     `gorm:"column:create_user_id" json:"createUserId"`
	CreateDate   time.Time `gorm:"column:create_date;type:datetime(3)" json:"createDate"`
	ModifyUserID int64     `gorm:"column:modify_user_id" json:"modifyUserId"`
	ModifyDate   time.Time `gorm:"column:modify_date;type:datetime(3)" json:"modifyDate"`
	DeleteMark   int       `gorm:"column:delete_mark;not null" json:"deleteMark"`
	EnabledMark  int       `gorm:"column:enabled_mark;not null" json:"enabledMark"`
	TenantID     string    `gorm:"column:tenant_id;type:varchar(255)" json:"tenantId"`
}

func (XjrUser) TableName() string {
	return "xjr_user"
}
