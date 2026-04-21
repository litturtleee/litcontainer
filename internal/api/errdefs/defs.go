package errdefs

// 服务端错误
const (
	ErrInternalServerError = "ErrInternalServerError"
)

// 容器相关错误码
const (
	// 容器id无效
	ErrInvalidContainerID = "ErrInvalidContainerID"
	// 容器不存在
	ErrContainerNotFound = "ErrContainerNotFound"

	// 容器已存在（创建时冲突）
	ErrContainerExists = "ErrContainerExists"

	// 容器正在运行（无法执行停止外的操作，如删除）
	ErrContainerRunning = "ErrContainerRunning"

	// 容器已停止（无法执行启动外的操作，如暂停）
	ErrContainerStopped = "ErrContainerStopped"

	// 容器配置无效（如端口格式错误、命令不存在）
	ErrInvalidContainerConfig = "ErrInvalidContainerConfig"

	// 容器依赖的镜像不存在
	ErrContainerImageNotFound = "ErrContainerImageNotFound"

	// 容器端口已被占用
	ErrContainerPortInUse = "ErrContainerPortInUse"

	// 容器操作超时（如启动超时、健康检查超时）
	ErrContainerTimeout = "ErrContainerTimeout"
)

// 镜像相关错误码
const (
	// 镜像不存在
	ErrImageNotFound = "ErrImageNotFound"

	// 镜像已存在（拉取或构建时冲突）
	ErrImageExists = "ErrImageExists"

	// 镜像标签无效（如格式错误、包含非法字符）
	ErrInvalidImageTag = "ErrInvalidImageTag"

	// 镜像拉取失败（如仓库不可达、权限不足）
	ErrImagePullFailed = "ErrImagePullFailed"

	// 镜像推送失败（如无推送权限、仓库不存在）
	ErrImagePushFailed = "ErrImagePushFailed"

	// 镜像构建失败（如 Dockerfile 语法错误、依赖缺失）
	ErrImageBuildFailed = "ErrImageBuildFailed"
)

// 网络相关错误码
const (
	// 网络不存在
	ErrNetworkNotFound = "ErrNetworkNotFound"

	// 网络已存在（创建时冲突）
	ErrNetworkExists = "ErrNetworkExists"

	// 网络正在使用（无法删除）
	ErrNetworkInUse = "ErrNetworkInUse"

	// 网络配置无效（如子网冲突、驱动不支持）
	ErrInvalidNetworkConfig = "ErrInvalidNetworkConfig"

	// 网络驱动不存在（如指定了未安装的驱动）
	ErrNetworkDriverNotFound = "ErrNetworkDriverNotFound"

	// 容器已连接到网络（重复连接时冲突）
	ErrContainerAlreadyConnected = "ErrContainerAlreadyConnected"

	// 容器未连接到网络（断开连接时错误）
	ErrContainerNotConnected = "ErrContainerNotConnected"
)

// 登录鉴权相关错误码
const (
	// 参数错误
	ErrInvalidParameter = "ErrInvalidParameter"

	// 登录失败
	ErrLoginFailed = "ErrLoginFailed"

	// 注册失败
	ErrRegisterFailed = "ErrRegisterFailed"

	// 权限不足
	ErrUnauthorized = "ErrUnauthorized"

	// 登出失败
	ErrLogoutFailed = "ErrLogoutFailed"

	// 认证失败
	ErrAuthFailed = "ErrAuthFailed"

	// 认证服务不可用
	ErrAuthServiceUnavailable = "ErrAuthServiceUnavailable"

	// 认证服务类型错误
	ErrAuthServiceTypeInvalid = "ErrAuthServiceTypeInvalid"

	// 无效的用户名或密码
	ErrInvalidUsernameOrPassword = "ErrInvalidUsernameOrPassword"

	// 用户未登录
	ErrNotLoggedIn = "ErrNotLoggedIn"

	// 登录凭证无效（如用户名或密码错误）
	ErrInvalidCredentials = "ErrInvalidCredentials"

	// 访问被拒绝（如权限不足、资源受限）
	ErrAccessDenied = "ErrAccessDenied"

	// 会话已过期（需要重新登录）
	ErrSessionExpired = "ErrSessionExpired"

	// 令牌无效（如 JWT 令牌错误）
	ErrInvalidToken = "ErrInvalidToken"
)
