package db

import (
	"fmt"
	"jos-deployment/pkg/model"
	"log"
	"os"
	"path/filepath"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// 数据库配置常量
const (
	DBHost     = "join-mysql-standalone-svc:3306"
	DBUser     = "root"
	DBPassword = "zn123$%^zn"
	DBName     = "user_center_workspace"
)

// 数据库服务封装
type Database struct {
	JosDb    *gorm.DB
	SqliteDb *gorm.DB
}

var DB Database

func InitDB(path string) error {
	err := initJosDB()
	if err != nil {
		return fmt.Errorf("failed to initialize MySQL database: %w", err)
	}

	return initSqliteDB(path)
}

func initJosDB() error {
	// 构建DSN (Data Source Name)
	dsn := fmt.Sprintf("%s:%s@tcp(%s)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		DBUser, DBPassword, DBHost, DBName)

	// 配置GORM日志
	newLogger := logger.New(
		log.New(log.Writer(), "jos-database\r\n", log.LstdFlags),
		logger.Config{
			SlowThreshold: time.Second,
			LogLevel:      logger.Info,
			Colorful:      true,
		},
	)

	// 连接 jos 数据库
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{
		Logger: newLogger,
	})
	if err != nil {
		return fmt.Errorf("failed to connect database: %w", err)
	}
	// 测试连接
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	if err := sqlDB.Ping(); err != nil {
		return fmt.Errorf("failed to ping database: %v", err)
	}

	// 设置连接池参数
	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetMaxOpenConns(100)
	sqlDB.SetConnMaxLifetime(time.Hour)

	log.Println("Successfully connected to MySQL database")
	DB.JosDb = db
	return nil
}

// 初始化数据库连接
func initSqliteDB(dbPath string) error {
	// 确保目录存在
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		log.Fatalf("创建目录失败: %v", err)
	}
	// 配置GORM日志
	newLogger := logger.New(
		log.New(log.Writer(), "sqlite-database\r\n", log.LstdFlags),
		logger.Config{
			SlowThreshold: time.Second,
			LogLevel:      logger.Info,
			Colorful:      true,
		},
	)
	// 打开数据库连接
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{
		Logger: newLogger,
	})
	if err != nil {
		return fmt.Errorf("failed to connect database: %w", err)
	}

	// 自动迁移表结构
	if err := db.AutoMigrate(&model.ProxyUserApp{}); err != nil {
		return fmt.Errorf("failed to migrate database: %w", err)
	}

	DB.SqliteDb = db
	return nil
}

// 关闭数据库连接
func (d *Database) Close() error {
	sqlDB, err := d.SqliteDb.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

// 创建用户应用关联
func (d *Database) CreateProxyUserApp(userApp *model.ProxyUserApp) error {
	if userApp.ID == 0 {
		return d.SqliteDb.Create(userApp).Error
	}
	return d.SqliteDb.Save(userApp).Error
}

// 更具UserName更新sqlite
func (d *Database) UpdateProxyUser(userApps []model.ProxyUserApp) error {
	// 检查UserName是否存在
	userName := userApps[0].UserName
	exists, err := d.CheckProxyUserNameExists(userName)
	if err != nil {
		return fmt.Errorf("failed to check user name existence: %w", err)
	}
	if exists {
		// 如果存在，则删除记录
		if err := d.SqliteDb.Where("user_name = ?", userName).Delete(&model.ProxyUserApp{}).Error; err != nil {
			return fmt.Errorf("failed to delete existing user app: %w", err)
		}
	}
	for _, userApp := range userApps {
		if err := d.CreateProxyUserApp(&userApp); err != nil {
			return fmt.Errorf("failed to create user app: %w", err)
		}
	}
	return nil
}

// 检测UserName是否存在
func (d *Database) CheckProxyUserNameExists(userName string) (bool, error) {
	var count int64
	if err := d.SqliteDb.Model(&model.ProxyUserApp{}).Where("user_name = ?", userName).Count(&count).Error; err != nil {
		return false, fmt.Errorf("failed to check user name existence: %w", err)
	}
	return count > 0, nil
}

// 根据UserName获取应用列表
func (d *Database) GetAppsByUserID(userID uint64) ([]model.ProxyUserApp, error) {
	var apps []model.ProxyUserApp
	if err := d.SqliteDb.Where("user_id = ?", userID).Find(&apps).Error; err != nil {
		return nil, fmt.Errorf("failed to get apps by user ID %d: %w", userID, err)
	}
	return apps, nil
}

func (d *Database) PutJosUserApp(userApp *model.JosUserApp) error {
	if userApp.ID == 0 {
		return d.JosDb.Create(userApp).Error
	}
	return d.JosDb.Save(userApp).Error
}

func (d *Database) CheckConnection() error {
	josDB, err := d.JosDb.DB()
	if err != nil {
		return err
	}
	if err := josDB.Ping(); err != nil {
		return fmt.Errorf("failed to ping JOS database: %v", err)
	}

	sqlDB, err := d.SqliteDb.DB()
	if err != nil {
		return err
	}
	return sqlDB.Ping()
}

func (d *Database) GetJosAppIDByID(id uint64) (uint64, error) {
	var app model.JosApp
	if err := d.JosDb.Select("app_id").Where("id = ?", id).First(&app).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return 0, fmt.Errorf("app with ID %d not found", id)
		}
		return 0, fmt.Errorf("failed to query app with ID %d: %w", id, err)
	}
	return app.AppID, nil
}

func (d *Database) GetUserByID(userID uint64) (model.XjrUser, error) {
	var user model.XjrUser
	if err := d.JosDb.First(&user, userID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return model.XjrUser{}, fmt.Errorf("user with ID %d not found", userID)
		}
		return model.XjrUser{}, fmt.Errorf("failed to get user by ID %d: %w", userID, err)
	}
	return user, nil
}
