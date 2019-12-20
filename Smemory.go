package Session

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"time"
)

// about Session or Cookie Func

// Cookie
// 从浏览器获取设置的cookieKey:uuid

// 封装￿ => 动态生成uid在local session创建对应数据 并在浏览器设置cookie
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
