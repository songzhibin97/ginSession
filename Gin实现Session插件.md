### Gin实现Session插件

#### `define.go` 主要定义一些必要的结构体以及接口 并且声明一些全局变量以供使用
``` go
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
```

#### `Smemory.go` 主要实现Session内存化 规范与session接口 以及一些开放的方法供用户使用
```go
package Session

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"time"
)

// about Session or Cookie Func

// Cookie
// 从浏览器获取设置的cookieKey:uuid

// 封装 => 动态生成uid在local session创建对应数据 并在浏览器设置cookie
func (g *GlobalMode) CreateCookieAndSession(cookieKey string, maxAge int, path, domain string, secure, httpOnly bool,
	limitation time.Duration, c *gin.Context) (Nuid string, err error) {
	// 设置浏览器cookie 并在local session设置对应uid的map
	Nuid, err = GetUuid()
	if err != nil {
		return "", err
	}
	// 设置cookie
	c.SetCookie(cookieKey, Nuid, maxAge, path, domain, secure, httpOnly)
	// 调用设置session
	g.NewSession(Nuid, limitation)
	return Nuid, nil
}

// Sessions

// 在local session仓中新建一条对应数据
func (g *GlobalMode) NewSession(uid string, limitation time.Duration) {
	// 加写锁
	g.Lock.Lock()
	// 注册解锁
	defer g.Lock.Unlock()
	// uid 对应cookie唯一标识 limitation 对应本地session有效时间
	st := make(map[string]interface{})
	(*g.Local)[uid] = &STemporary{
		ST:        &st,
		Effective: time.Now().Add(limitation),
	}
}

// 在local session仓中查找对应uid的数据
func (g *GlobalMode) FindSessionMap(uid string) (tsObj *STemporary, err error) {
	// 加读锁
	g.Lock.RLock()
	// 注册解锁
	defer g.Lock.RUnlock()
	tsObj, ok := (*g.Local)[uid]
	if !ok {
		err = fmt.Errorf("Pleace Check 'uid' is exist or valid\n")
		return nil, err
	}
	return tsObj, nil
}

// 在local session仓增加对应uid下TsObj的有效时间
func (g *GlobalMode) AddTsTime(uid string, limitation time.Duration) error {
	// 调用查找方法
	tsObj, err := g.FindSessionMap(uid)
	if err != nil {
		return err
	}
	// 进行local session 时长修改
	tsObj.Effective = time.Now().Add(limitation)
	return nil
}

// 在local session仓删除对应uid的数据
func (g *GlobalMode) DeleteSessionUid(uid string) {
	// 加写锁
	g.Lock.Lock()
	// 注册解锁
	defer g.Lock.Unlock()
	delete(*g.Local, uid)
}

// 在local session仓库对应uid的TS增加键值对
func (g *GlobalMode) SetSTKeyValue(uid, key string, value interface{}) (err error) {
	// 调用查找 验证有效性
	tsObj, err := g.FindSessionMap(uid)
	if err != nil {
		return
	}
	// 加写锁
	tsObj.Lock.Lock()
	// 注册解锁
	defer tsObj.Lock.Unlock()
	(*tsObj.ST)[key] = value
	return nil
}

// 在local session仓库对应uid的TS查找对应键的值
func (g *GlobalMode) FindSTKey(uid, key string) (value interface{}, err error) {
	// 调用查找 验证有效性
	tsObj, err := g.FindSessionMap(uid)
	if err != nil {
		return nil, err
	}
	// 加读锁
	tsObj.Lock.RLock()
	// 注册解锁
	defer tsObj.Lock.RUnlock()
	value, ok := (*tsObj.ST)[key]
	if !ok {
		err = fmt.Errorf("Pleace Check 'key' is exist or valid\n")
		return
	}
	return value, nil
}

// 在local session仓库对应uid的TS删除对应键的值
func (g *GlobalMode) DeleteSTKey(uid, key string) {
	// 调用查找 验证有效性
	tsObj, err := g.FindSessionMap(uid)
	if err != nil {
		return
	}
	// 加写锁
	tsObj.Lock.Lock()
	// 注册解锁
	defer tsObj.Lock.Unlock()
	delete(*tsObj.ST, key)
}

// 续时 如果用于以存在cookie 进行浏览器和本地session仓增加有效时间
func (g *GlobalMode) AddValidTime(cookieKey string, uid string, maxAge int, path, domain string, secure, httpOnly bool,
	limitation time.Duration, c *gin.Context) error {
	// 重新设置cookie 增加有效时长
	c.SetCookie(cookieKey, uid, maxAge, path, domain, secure, httpOnly)
	// 在本地session仓中给对应的增加有效时间
	err := g.AddTsTime(uid, limitation)
	if err != nil {
		return err
	}
	return nil
}

// 建议起goroutine循环执行
// 设置定时任务 周期执行cookie清除 // d=>循环周期
func (g *GlobalMode) CycleCheck(d time.Duration) {
	// 创建定时任务
	ticker := time.Tick(d)
	for range ticker {
		fmt.Println("开始执行清理任务")
		g.ClearSession()
		for key, value := range *Memory.Local {
			fmt.Printf("uuid:%v=>ts:%v\n", key, *value.ST)
		}

	}
}

// 清理local 已失效的
func (g *GlobalMode) ClearSession() {
	for uid, st := range *g.Local {
		if st.Effective.Before(time.Now()) {
			// 超时处理 调用删除session接口
			g.DeleteSessionUid(uid)
		}
	}
}

// Middle Ware
// gin框架中间件 需要注册在首部保证cookie正常使用
// cookieKey 在浏览器设置cookie的key:uuid
func (g *GlobalMode) MiddleWare(cookieKey string, maxAge int, path, domain string, secure, httpOnly bool,
	limitation time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 调用查询 检测是否有cookieKey
		uid, err := c.Cookie(cookieKey)
		if err != nil {
			// 检测失败如果未设置cookie 调用封装方法 设置cookie并创建对应session
			uid, err := g.CreateCookieAndSession(cookieKey, maxAge, path, domain, secure, httpOnly, limitation, c)
			if err != nil {
				fmt.Println("Set Cookie and Session Error:", err)
				return
			}
			c.Set("UID", uid)
			for key, value := range *Memory.Local {
				fmt.Printf("uuid:%v=>ts:%v\n", key, *value.ST)
			}
			return
		}
		// 获取到浏览器的cookie 调用查找验证uid有效性
		_, err = g.FindSessionMap(uid)
		if err != nil {
			// 检测失败 无效uid 进行重新设置 调用封装方法 设置cookie 并创建对应session
			uid, err := g.CreateCookieAndSession(cookieKey, maxAge, path, domain, secure, httpOnly, limitation, c)
			if err != nil {
				fmt.Println("Set Cookie and Session Error:", err)
				return
			}
			c.Set("UID", uid)
			for key, value := range *Memory.Local {
				fmt.Printf("uuid:%v=>ts:%v\n", key, *value.ST)
			}
			return
		}
		// 设置TS 上下文可取用设置对应key:value
		err = g.AddValidTime(cookieKey, uid, maxAge, path, domain, secure, httpOnly, limitation, c)
		if err != nil {
			fmt.Println("续时失败")
		}
		c.Set("UID", uid)
		for key, value := range *Memory.Local {
			fmt.Printf("uuid:%v=>ts:%v\n", key, *value.ST)
		}
		return
	}
}
```

#### `Sredis.go`主要实现Session存放至Redis中 规范与session接口 以及一些开放的方法供用户使用
```go
package Session

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"time"
)

// 封装 => 动态生成uid在local session创建对应数据 并在浏览器设置cookie
func (r *SRedis) CreateCookieAndSession(cookieKey string, maxAge int, path, domain string, secure, httpOnly bool,
	limitation time.Duration, c *gin.Context) (Nuid string, err error) {
	// 设置浏览器cookie 并在local session设置对应uid的map
	Nuid, err = GetUuid()
	if err != nil {
		return "", err
	}
	// 设置cookie
	c.SetCookie(cookieKey, Nuid, maxAge, path, domain, secure, httpOnly)
	// 调用设置redis session
	r.NewSession(Nuid, limitation)
	return Nuid, nil
}

// 在redis 仓中新建一条对应数据
func (r *SRedis) NewSession(uid string, limitation time.Duration) {
	r.client.HSet(uid, "isRedis", true)
	r.client.Expire(uid, limitation)
}

// 在redis仓中查找对应uid的数据
func (r *SRedis) FindSessionMap(uid string) (keys []string, err error) {
	keys, err = r.client.HKeys(uid).Result()
	if err != nil {
		return
	}
	return
}

// 在redis仓根据uid key 获取对应value
func (r *SRedis) FindSTKey(uid, key string) (value string, err error) {
	value, err = r.client.HGet(uid, key).Result()
	if err != nil {
		return "", err
	}
	return value, nil
}

// 续时 如果用于以存在cookie 进行浏览器和本地session仓增加有效时间
func (r *SRedis) AddValidTime(cookieKey string, uid string, maxAge int, path, domain string, secure, httpOnly bool,
	limitation time.Duration, c *gin.Context) error {
	// 重新设置cookie 增加有效时长
	c.SetCookie(cookieKey, uid, maxAge, path, domain, secure, httpOnly)
	// 在本地session仓中给对应的增加有效时间
	r.client.Expire(uid, limitation)
	return nil
}

func (r *SRedis) SetSTKeyValue(uid, key string, value interface{}) error {
	r.client.HSet(uid, key, value)
	return nil
}

func (r *SRedis) CycleCheck(d time.Duration) {
	fmt.Println("Redis no need this func", d)
}
func (r *SRedis) MiddleWare(cookieKey string, maxAge int, path, domain string, secure, httpOnly bool,
	limitation time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid, err := c.Cookie(cookieKey)
		if err != nil {
			// 检测失败 如果未设置cookie 调用封装方法 设置cookie并在redis创建对应session
			uid, err = r.CreateCookieAndSession(cookieKey, maxAge, path, domain, secure, httpOnly, limitation, c)
			if err != nil {
				fmt.Println("Set Cookie and Session Error:", err)
				return
			}
			c.Set("UID", uid)
			return
		}
		// 获取到浏览器cookie 调用查找验证uid有效性
		_, err = r.FindSessionMap(uid)
		if err != nil {
			// 检测失败 无效uid 进行重设 调用封装方法 设置cookie 并创建对应session
			uid, err = r.CreateCookieAndSession(cookieKey, maxAge, path, domain, secure, httpOnly, limitation, c)
			if err != nil {
				fmt.Println("Set Cookie and Session Error:", err)
				return
			}
			c.Set("UID", uid)
			return
		}
		// 动态设置延时
		err = r.AddValidTime(cookieKey, uid, maxAge, path, domain, secure, httpOnly, limitation, c)
		if err != nil {
			fmt.Println("续时失败")
		}
		c.Set("UID", uid)
		return
	}
}
```


#### Installation
```shell
go get github.com/songzhibin97/ginSession
```

#### Related Projects
```go
// memory
package main

import (
	"Songzhibin/Session"
	"fmt"
	"github.com/gin-gonic/gin"
	"net/http"
	"time"
)

func main() {
	Session.Init("") // init传入"" or "memory" 即可开启memory版session
	go Session.Warehouse.CycleCheck(time.Minute) // 起goroutine循环执行清理功能(本地session仓过期的uid数值)
	r := gin.Default()
	r.Use(Session.Warehouse.MiddleWare("Logoer", 200, "", "127.0.0.1", false, true, 2*time.Minute)) // 注册 package session实现的中间件方法 Session.Warehouse 适用于所有 通用接口类
	r.GET("/memory", func(c *gin.Context) {
		id, ok := c.Get("UID") // 获取session.Warehouse.MiddleWare生成的UID
		if !ok {
			fmt.Println("No fount `UID`")
		}
		err := Session.Warehouse.SetSTKeyValue(id.(string), "sb", 123) // 在session仓添加key:value键值对
		if err != nil {
			fmt.Println("设置session失败:",err)
		}
		c.JSON(http.StatusOK, gin.H{"message": "Hello world!"})
	})
}


// redis
import (
	"Songzhibin/Session"
	"fmt"
	"github.com/gin-gonic/gin"
	"net/http"
	"time"
)

func main() {
	Session.Init("redis") // init传入redis 即可开启redis版session
	r := gin.Default()
	r.Use(Session.Warehouse.MiddleWare("Logoer", 200, "", "127.0.0.1", false, true, 2*time.Minute)) // 注册 package session实现的中间件方法 Session.Warehouse 适用于所有 通用接口类
	r.GET("/memory", func(c *gin.Context) {
		id, ok := c.Get("UID") // 获取session.Warehouse.MiddleWare生成的UID
		if !ok {
			fmt.Println("No fount `UID`")
		}
		err := Session.Warehouse.SetSTKeyValue(id.(string), "sb", 123) // 在redis仓添加key:value键值对
		if err != nil {
			fmt.Println("设置session失败:",err)
		}
		c.JSON(http.StatusOK, gin.H{"message": "Hello world!"})
	})
}

