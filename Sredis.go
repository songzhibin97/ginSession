package Session

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"time"
)

// 封装￿ => 动态生成uid在local session创建对应数据 并在浏览器设置cookie
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
