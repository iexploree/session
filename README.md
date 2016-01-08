copy 自谢大的 session:  https://github.com/astaxie/session

修改了一些 bug:

1. 修改 BUG, 作者没有使用 QueryUnescape 转义，导致无法销毁 session
	func (manager *Manager) SessionDestroy()

2. SessionRead 只读，不存在时不创建，避免 SessionStart 收到不存在的 cookie 时
	错误的使用收到的 cookie 进行创建。