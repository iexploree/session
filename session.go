package session

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"sync"
	"time"
)

type Session interface {
	Set(key, value interface{}) error //set session value
	Get(key interface{}) interface{}  //get session value
	Delete(key interface{}) error     //delete session value
	SessionID() string                //back current sessionID
}

type Provider interface {
	SessionInit(sid string) (Session, error)
	SessionRead(sid string) (Session, error)
	SessionDestroy(sid string) error
	SessionGC(maxlifetime int64)
}

var provides = make(map[string]Provider)

// Register makes a session provide available by the provided name.
// If Register is called twice with the same name or if driver is nil,
// it panics.
func Register(name string, provide Provider) {
	if provide == nil {
		panic("session: Register provide is nil")
	}
	if _, dup := provides[name]; dup {
		panic("session: Register called twice for provide " + name)
	}
	provides[name] = provide
}

type Manager struct {
	cookieName  string     //private cookiename
	lock        sync.Mutex // protects session
	provider    Provider
	maxlifetime int64
}

func NewManager(provideName, cookieName string, maxlifetime int64) (*Manager, error) {
	provider, ok := provides[provideName]
	if !ok {
		return nil, fmt.Errorf("session: unknown provide %q (forgotten import?)", provideName)
	}
	return &Manager{provider: provider, cookieName: cookieName, maxlifetime: maxlifetime}, nil
}

//get Session
func (manager *Manager) SessionStart(w http.ResponseWriter, r *http.Request) (session Session) {
	manager.lock.Lock()
	defer manager.lock.Unlock()
	cookie, err := r.Cookie(manager.cookieName)

	if err != nil || cookie.Value == "" {
		log.Println("cookie is empty, new session.")

		sid := manager.sessionId()
		session, _ = manager.provider.SessionInit(sid)
		/**
		设置cookie的httponly为true,这个属性是设置是否可通过客户端脚本访问这个设置的cookie，
		第一这个可以防止这个cookie被XSS读取从而引起session劫持，
		第二cookie设置不会像URL重置方式那么容易获取sessionID。
		*/
		cookie := http.Cookie{Name: manager.cookieName, Value: url.QueryEscape(sid), Path: "/", HttpOnly: true, MaxAge: int(manager.maxlifetime)}
		http.SetCookie(w, &cookie)
	} else {
		sid, _ := url.QueryUnescape(cookie.Value)
		log.Println("cookie value:", cookie.Value, " sid:", sid)

		// yinfeng: 修改 BUG ，修改 SessionRead 如果不存在，不创建新的返回
		var readerr error
		session, readerr = manager.provider.SessionRead(sid)
		if readerr != nil {
			// 如果不存在session, 则创建一个新的
			log.Println("new session")
			sid := manager.sessionId()
			session, _ = manager.provider.SessionInit(sid)
			cookie := http.Cookie{Name: manager.cookieName, Value: url.QueryEscape(sid), Path: "/", HttpOnly: true, MaxAge: int(manager.maxlifetime)}
			http.SetCookie(w, &cookie)
		}

	}
	return
}

//Destroy sessionid
func (manager *Manager) SessionDestroy(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(manager.cookieName)
	if err != nil || cookie.Value == "" {
		return
	} else {
		manager.lock.Lock()
		defer manager.lock.Unlock()
		// yinfeng: 修改 BUG, 作者没有使用 QueryUnescape 转义，导致无法销毁 session
		sid, _ := url.QueryUnescape(cookie.Value)
		manager.provider.SessionDestroy(sid)
		expiration := time.Now()
		// MaxAge 设置为 -1 删除客户端的 cookie 缓存
		cookie := http.Cookie{Name: manager.cookieName, Path: "/", HttpOnly: true, Expires: expiration, MaxAge: -1}
		http.SetCookie(w, &cookie)
	}
}

func (manager *Manager) GC() {
	manager.lock.Lock()
	defer manager.lock.Unlock()
	manager.provider.SessionGC(manager.maxlifetime)
	time.AfterFunc(time.Duration(manager.maxlifetime)*time.Second, func() { manager.GC() })
}

func (manager *Manager) sessionId() string {
	b := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		return ""
	}
	return base64.URLEncoding.EncodeToString(b)
}
