package Session

import (
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis"
	uuid "github.com/satori/go.uuid"
	"sync"
	"time"
)

// 创建接口 规范化Session存储位置
type Session interface {
	MiddleWare(cookieKey string, maxAge int, path, domain string, secure, httpOnly bool,
		limitation time.Duration) gin.HandlerFunc           //必须要有中间件方法
	SetSTKeyValue(uid, key string, value interface{}) error // 必须要有添加key:value方法
	CycleCheck(d time.Duration)
}

// 定义全局Session模型 // 仅在Local模式下启用
type GlobalMode struct {
	Local *map[string]*STemporary
	Lock  sync.RWMutex
}

// 定义临时存储Session ST维护一个临时map ==> key:value类型
// Lock 加锁
type STemporary struct {
	ST        *map[string]interface{}
	Effective time.Time
	Lock      sync.RWMutex
}

// 定义redis存储模型
type SRedis struct {
	// 维护一个redis连接池
	client *redis.Client
}


// 声明全局变量Session模型 并未初始化 需要调用 Init 初始化操作
var Memory GlobalMode

// 声明全局变量SRedis模型 并未初始化 需要调用 Init 初始化操作
var LRedis SRedis

// 创建接口变量
var Warehouse Session

// 初始化操作
func Init(name string, other ...interface{}) {
	switch name {
	case "", "memory":
		// 初始化条件
		// 初始化local
		lc := make(map[string]*STemporary)
		Memory.Local = &lc
		Warehouse = &Memory
	case "redis":

		// 初始化条件 需要获取other中对应的参数
		// other接受 1?2?3...个参数
		// 顺序分别为 Addr - Password - DB
		var addr string
		var password = ""
		var db = 0
		switch len(other) {
		// 动态判断other参数 分配参数值
		case 0:
			panic("init error: need some parameter example `Addr=>localhost:port?`")
		case 1:
			addr = other[0].(string)
		case 2:
			addr, password = other[0].(string), other[1].(string)
		case 3:
			addr, password, db = other[0].(string), other[1].(string), other[2].(int)
		}
		// 初始化RLocal
		LRedis.client = redis.NewClient(&redis.Options{
			// 将LRedis.client赋值连接池
			Addr:     addr,
			Password: password,
			DB:       db,
		})
		_, err := LRedis.client.Ping().Result()
		if err != nil {
			panic("not connect successfully Redis please check `add` other")
		}
		Warehouse = &LRedis
	default:
		panic("understand your input")
	}
}

// ================= 公共方法
// 创建UUID
func GetUuid() (uid string, err error) {
	Nuid, err := uuid.NewV4()
	if err != nil {
		// 获取uuid失败
		return "", err
	}
	return Nuid.String(), nil
}
