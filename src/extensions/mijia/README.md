# mijia：板端独立米家 CLI

这是可选的 PicoClaw 扩展，不链接进主应用，也不注册米家专用模型工具。仅使用 Go 标准库，`CGO_ENABLED=0` 编译为 ARMv7 Linux 纯二进制；不需要 Python、Node.js、MiLoCo 后端或 PC 常驻服务。云协议参考仓库中的 HachimoDock Rust 实现。

支持 OAuth 授权和自动刷新、账号设备/房间与已有手动场景同步、MIOT 规格查询、属性读取/设置、动作及场景执行。当前只支持中国大陆云端接口；未移植摄像头、感知、规则调度、本地中枢、MQTT 属性订阅。需要网络及账号授权的设备能力。

## 构建与安装

在 `src/` 下执行：

```sh
tools/dev.sh sh tools/build-mijia.sh arm
```

产物为 `build/arm/mijia` 和 `build/mijia-extension-armv7.tar.gz`。可在电脑解压扩展包后，将其中 `agent/` 合并上传到共享，再应用配置；也可逐项上传：

1. 将二进制上传到共享中的 `agent/bin/mijia`。
2. 将本目录 `SKILL.md` 上传为 `agent/skills/mijia/SKILL.md`。技能与 CLI 可独立更新或删除。
3. 同目录上传 `Xiaomi-Miloco-LICENSE.md` 和 `THIRD_PARTY_NOTICES.md`，保留来源与许可。
4. 在设置 → 系统选择“应用配置并重启”。程序会恢复 CLI 执行权限。

之后智能体通过已有 `exec` 调用 `mijia`。它的实现没有写进 PicoClaw 的 Go 工具注册。

## 首次授权

在板端命令行运行，或让智能体执行：

```sh
mijia auth login
```

返回 JSON 中包含小米官方 OAuth 网页 URL；在手机或电脑浏览器打开，完成授权，保存授权页给出的结果或回调链接到文本文件。登录请求有效期 10 分钟。浏览器只负责一次性授权，不运行后端。

将文本文件通过 SMB 上传到 `secrets/mijia-auth.txt`，可直接从共享的编辑区导入：

```sh
mijia auth authorize --file /opt/hachimodock/config-share/secrets/mijia-auth.txt
mijia devices refresh
mijia devices list
```

返回结果不包含账号令牌。完成后从共享中删除 `secrets/mijia-auth.txt`。此临时文件导入无需应用配置或重启。也可在板端直接使用私有临时文件导入，授权文件内容无需发送给模型。

`exec` 自动设置私有状态路径，令牌默认放在 `workspace/cli-state/mijia/`，目录 0700、文件 0600，不在 SMB 的配置分享目录中。终端手动运行时，为与智能体共享同一账号，设置 `MIJIA_STATE_DIR` 为实际应用数据目录下的 `workspace/cli-state/mijia`。未设置时使用 `HACHIMODOCK_CLI_STATE_DIR/mijia`，最后回退到当前用户配置目录的 `mijia-lite/`。用 `mijia auth logout` 删除本机令牌、设备缓存及待授权记录。

## 命令与结果

```sh
mijia auth status
mijia devices refresh
mijia devices list
mijia device spec DEVICE_ID
mijia device get DEVICE_ID
# 仅当目标设备的 spec 确认 SIID=2、PIID=1 为可写 bool 时：
mijia device set DEVICE_ID 2 1 true
# 按实际动作定义传入 JSON 数组：
mijia device action DEVICE_ID SIID AIID '[]'
mijia scenes list
mijia scenes run SCENE_ID
```

stdout 输出 `{"ok":true,"data":...}`；错误写入 stderr 的 JSON，退出码为 1。`--help` 显示命令帮助。`devices list` 读取本地缓存，`devices refresh` 更新云端清单并清空能力缓存。

控制前校验账号设备范围、能力 ID、访问权限、JSON 类型、枚举和范围；动作参数按规格顺序校验，缺失定义时拒绝猜测。写入可读属性后读回，值匹配时状态为 `verified`，否则为 `accepted`。动作及场景只表示受理。控制请求不自动重试、不自动回滚；退出或取消不能撤回云端已经收到的命令。

每次 HTTP 请求上限 25 秒、响应最多 4 MiB，进程总时限 120 秒；智能体默认 exec 超时 60 秒会优先结束进程。并发进程通过本地文件锁串行化，状态使用临时文件原子替换。TLS 使用 Go 默认的系统证书校验，禁止接口重定向。设备列表按白名单提取字段，不向模型返回 LAN token、IP、SSID 或账号 UID。

## 验证范围

```sh
tools/dev.sh sh -c 'cd extensions/mijia && go test -race ./... && go vet ./...'
```

协议模拟测试覆盖真实 AES/RSA 加解密、OAuth state、令牌刷新、设备/能力解析、字符串/数字混合的大整数 ID 分页及缓存、参数校验、受理/读回状态及不重发控制。自动测试不连接真实米家账号、不发送真实设备命令。本板已完成真实 OAuth 授权和设备/场景同步；物理设备控制尚未实机验收。
