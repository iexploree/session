package memory

import (
	"container/list"
	"github.com/iexploree/session"
	"log"
	"sync"
	"time"
)

type SessionError string

func (err SessionError) Error() string { return "Session Error: " + string(err) }

var DoesNotExist = SessionError("Key does not exist ")

var pder = &Provider{list: list.New()}

// 实现了 session.Session 接口的方法
type SessionStore struct {
	sid          string                      //session id唯一标示
	timeAccessed time.Time                   //最后访问时间
	value        map[interface{}]interface{} //session里面存储的值
}

/**
对于 SessionStore 的 Set Get Delete 操作都会更新 session 链表，
把这个 SessionStore 移到链表头部
*/
func (st *SessionStore) Set(key, value interface{}) error {
	st.value[key] = value
	pder.SessionUpdate(st.sid)
	return nil
}

func (st *SessionStore) Get(key interface{}) interface{} {
	pder.SessionUpdate(st.sid)
	if v, ok := st.value[key]; ok {
		return v
	} else {
		return nil
	}
	return nil
}

func (st *SessionStore) Delete(key interface{}) error {
	delete(st.value, key)
	pder.SessionUpdate(st.sid)
	return nil
}

func (st *SessionStore) SessionID() string {
	return st.sid
}

type Provider struct {
	lock sync.Mutex //用来锁

	// 存储的 key 是 SessionStore 的 sid
	// 值是下面  list 的 *list.Element 元素
	// 方便快速查找 *list.Element 元素
	sessions map[string]*list.Element //用来存储在内存

	// list.Element Value 是 *SessionStore 类型
	// 越靠近链表尾部，是越老的 session
	list *list.List //用来做gc
}

/**
创建 session
*/
func (pder *Provider) SessionInit(sid string) (session.Session, error) {
	pder.lock.Lock()
	defer pder.lock.Unlock()
	v := make(map[interface{}]interface{}, 0)
	newsess := &SessionStore{sid: sid, timeAccessed: time.Now(), value: v}
	// 为什么是放到链表尾部？应该是放到链表头部啊!
	// 虽然 SessionStore Get Set  Delete 会更新链表，放到尾部，导致这个BUG被隐藏了
	// 但显然存在隐患
	//element := pder.list.PushBack(newsess)
	element := pder.list.PushFront(newsess)

	// 放到 map 表方便快速查找
	pder.sessions[sid] = element
	return newsess, nil
}

func (pder *Provider) SessionRead(sid string) (session.Session, error) {
	if element, ok := pder.sessions[sid]; ok {
		log.Println("SessionRead exist")
		return element.Value.(*SessionStore), nil
	}

	// yinfeng: 只做一件事，不做两件事
	// } else {
	// 	log.Println("SessionRead not exist, create new session")
	// 	sess, err := pder.SessionInit(sid)
	// 	return sess, err
	// }
	return nil, DoesNotExist
}

func (pder *Provider) SessionDestroy(sid string) error {
	if element, ok := pder.sessions[sid]; ok {
		log.Println("SessionDestroy success:", sid)
		delete(pder.sessions, sid)
		pder.list.Remove(element)
		return nil
	}
	log.Println("SessionDestroy failed:", sid)
	return nil
}

func (pder *Provider) SessionGC(maxlifetime int64) {
	pder.lock.Lock()
	defer pder.lock.Unlock()

	for {
		// 从尾部开始，因为越靠近链表尾部，是越老的 session
		element := pder.list.Back()
		if element == nil {
			break
		}

		// 老的 session，删除
		if (element.Value.(*SessionStore).timeAccessed.Unix() + maxlifetime) < time.Now().Unix() {
			pder.list.Remove(element)
			delete(pder.sessions, element.Value.(*SessionStore).sid)
			log.Println("delete session id:", element.Value.(*SessionStore).sid)
		} else {
			break
		}
	}
}

func (pder *Provider) SessionUpdate(sid string) error {
	pder.lock.Lock()
	defer pder.lock.Unlock()
	if element, ok := pder.sessions[sid]; ok {
		// 更新时间，移到链表头部
		element.Value.(*SessionStore).timeAccessed = time.Now()
		pder.list.MoveToFront(element)
		return nil
	}
	return nil
}

func init() {
	pder.sessions = make(map[string]*list.Element, 0)
	session.Register("memory", pder)
}
