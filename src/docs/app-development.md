# 应用开发规范

应用采用编译期注册，由一个进程运行。`internal/bootstrap/apps.go` 是具体应用及页面的装配入口，目录根据注册表生成。PicoClaw 是第一个完整实现，可以作为开发参照。

## 目录与依赖

```text
internal/
├── core/                         公共应用契约、注册和导航
├── bootstrap/
│   ├── apps.go                   注册应用、页面和终端命令
│   └── picoclaw.go               组装 PicoClaw 的具体服务
├── apps/
│   └── <id>/
│       ├── app.go                Manifest、状态和生命周期
│       ├── config.go             应用配置解码与验证
│       └── app_test.go           业务和资源清理测试
├── platform/                     共享设备能力
└── ui/lvgl/
    ├── ui.go                     公共 UI 循环与页面分发
    ├── bridge.c                  显示、触摸、目录、样式和事件队列
    ├── bridge.h                  公共桥接接口
    ├── ui_native.h               页面可复用的原生组件与字体
    ├── page_<id>.go              状态到页面的适配器
    ├── page_<id>.c               页面组件与输入事件
    ├── page_<id>.h               应用页面的原生接口
    └── page_<id>_unavailable.go   未启用 Cgo/LVGL 时的构建替代
```

- `core` 只依赖标准库，不认识任何具体应用、设备或界面。
- 应用可以使用 `core`、配置和共享服务，不能导入 UI、`bootstrap` 或另一个应用。共享业务抽取为独立服务，通过构造参数注入。
- 平台、语音和模型适配器不能反向依赖应用或 UI。
- UI 的公共循环不导入应用；只有 `page_<id>*` 页面适配器可以读取对应应用的状态。
- `bootstrap` 选择具体实现，创建应用工厂，注册页面并注入终端命令。

`internal/architecture/architecture_test.go` 会检查上述依赖方向。原生页面文件目前放在同一个 Cgo 包中，使用 `page_<id>` 前缀隔离；每个页面拥有自己的头文件，新增页面不需要修改公共 `bridge.h`。

## 1. 定义应用信息和状态

ID 使用小写字母开头，可包含小写字母、数字和连字符，最长 48 字符，例如 `usage-monitor`。ID 同时用于注册、导航、配置、数据目录和事件来源，注册后应保持稳定。

```go
var Manifest = core.Manifest{
    ID: "usage-monitor",
    Title: "订阅用量",
    Description: "查看模型服务订阅用量",
    Icon: "U",
    Capabilities: []core.Capability{core.Network},
}
```

能力声明用于说明服务需求，当前没有权限沙箱，也不会因为可选麦克风缺失而隐藏应用。应用应在自己的状态中表达能力是否可用，例如 PicoClaw 无声卡时仍支持文字输入。

业务状态使用应用自己的 Go 类型。通过锁保护写入，`Snapshot` 返回独立副本；切片、映射和指针指向的可变内容也必须复制。用递增的 `Revision` 通知界面更新，界面不能直接修改业务状态。

## 2. 实现应用生命周期

```go
type Application interface {
    Start(context.Context) error
    Stop()
    Handle(core.Event) error
    Snapshot() core.AppSnapshot
    Close() error
}
```

| 入口 | 实现约定 |
|---|---|
| 工厂 `New` | 首次打开时调用，加载本应用配置并注入依赖；失败时清理已创建资源 |
| `Start(ctx)` | 每次进入时调用，使用这次进入的 context；保留历史业务状态，创建必要的前台任务 |
| `Handle(event)` | 快速处理输入；耗时请求启动后台任务，不阻塞 UI |
| `Snapshot()` | 返回版本、通用状态说明和应用状态副本；不执行 I/O |
| `Stop()` | 每次离开时调用，立即使旧任务失效并发出取消，异步完成设备清理；可以重复调用 |
| `Close()` | 退出进程或启动失败时调用，等待任务退出并释放保留的客户端；必须可重复调用 |

Host 串行调用生命周期和事件入口；应用自己的后台任务与快照读取仍需加锁。`Start` 和 `Handle` 不得同步等待网络、模型或长时间设备操作。工厂可以构造客户端，但不应在创建过程中建立网络会话。

所有前台任务必须派生自 `Start` 的 context。Host 在切换和返回时先取消该 context，再调用 `Stop`。阻塞读写需要能随取消关闭，不能留下无限等待的 goroutine；`Close` 应用 WaitGroup 等机制确认退出。

同一应用的实例在离开后缓存，再次进入会复用该实例并得到新的 context。需要新建会话时由应用显式处理。长请求应使用轮次编号或等效机制，防止取消后迟到的结果更新状态。

当前 Host 管理前台应用，不提供常驻后台应用调度。若以后需要持续监控，应将其实现为有独立生命周期的共享服务，由 `bootstrap` 管理；不要依靠离开页面后遗留的 goroutine。

## 3. 定义事件与页面

应用定义自己的动作常量，例如 PicoClaw 的 `submit`、`voice.toggle` 和 `conversation.new`。页面发送 `core.Event{AppID: ID, Action: ..., Text: ...}`；Host 仅将事件转发给当前活动且 ID 相同的应用，过期页面事件会被拒绝。

```go
type Page interface {
    ID() string
    Mount() error
    Render(core.AppSnapshot) error
    Unmount()
}
```

页面在进入时 `Mount`，收到新快照后 `Render`，离开时 `Unmount`。所有方法都由框架在唯一 LVGL OS 线程调用。`Mount` 不创建业务服务，`Render` 只更新组件，`Unmount` 删除全部页面组件及绑定数据。公共目录会先恢复为活动屏幕，再删除旧页面。

Go 适配器从 `AppSnapshot.Data` 检查并取得对应应用的状态类型；`Status` 可直接用于页面提示。原生页面使用 `ui_native.h` 中的字体、颜色和按钮，利用 `dock_emit(id, action, text)` 发出事件。返回按钮的动作固定为 `shell.back`，目录的 `shell.open` 由公共框架生成。

不要从后台 goroutine 调用 C/LVGL，不要把 C 组件地址存入应用状态。`Unmount` 后的原生指针应清空，延迟回调不能继续访问它们。Cgo 字符串的申请和释放必须配对。

## 4. 配置与持久化

根配置仅负责公共显示、音频设置和应用配置入口：

```json
{
  "data_dir": "data",
  "ui": {"font": "assets/wqy-microhei.ttc"},
  "apps": {
    "usage-monitor": {"endpoint": "https://service.example"}
  }
}
```

在应用的 `config.go` 定义自己的配置结构，使用 `root.DecodeApp(ID, &cfg)` 解码并验证。相对路径使用 `root.Resolve(path)`；默认数据位置用 `root.AppDataDir(ID)`，结果为 `data_dir/apps/<id>/`。服务凭据继续使用权限为 600 的独立密钥文件，不进入状态快照或日志。

只有打开应用时才读取其配置和密钥。某个应用配置缺失或启动失败时，Host 回到目录并显示错误，其他应用仍可打开。

PicoClaw 兼容旧版顶层 `llm/asr/tts` 配置，自动在内存中转换到应用命名空间，并保留原数据目录。如果手工改成 `apps.picoclaw`，应同时设置该应用的 `data_dir` 指向原目录，或明确迁移已有工作区。不要在升级时静默切换历史数据位置。

## 5. 在装配入口注册

在 `internal/bootstrap/apps.go` 中添加注册项，工厂只在首次打开时执行：

```go
core.Registration{
    Manifest: usage.Manifest,
    New: func() (core.Application, error) {
        // 在这里加载应用配置、创建并注入所需服务。
        return newUsageApplication(cfg)
    },
}
```

将对应页面构造器加入 `Desktop.Pages`。如果页面遗漏或重复，LVGL 前端会在启动时明确报错。终端诊断需要快捷命令时，在 `Desktop.Commands[ID]` 中登记别名到应用动作的映射；空字符串表示普通文字输入。

新增应用无需修改 `core`、公共目录绘制、公共 UI 事件循环或构建脚本。`page_<id>.go` / `.c` 位于现有 Cgo 包，构建时自动包含；禁用 LVGL 的构建需要相应的替代实现。

## 6. 验证与部署

至少覆盖应用进入和重入、请求失败、执行中离开、取消后迟到结果、资源释放、状态快照独立性。设备依赖通过接口替换，测试不需要真实网络密钥。

```sh
# 在 src/ 下执行
tools/dev.sh sh -c 'go test -race -p 2 -tags stdjson,goolm ./... && go vet -tags stdjson,goolm ./...'
tools/dev.sh tools/build.sh native
tools/dev.sh tools/check-ui.sh
tools/dev.sh build/native/dock -config .local/config.json -preview build/catalog.png
tools/dev.sh build/native/dock -config .local/config.json -preview build/app.png -page <id>
tools/dev.sh tools/build.sh arm
tools/deploy.sh root@192.168.4.202
```

板上验证目录入口、横屏坐标、返回后再次进入、处理中返回和错误提示；涉及音视频时还要验证切换后的设备释放。部署保持既有配置和数据，不会替换 boot；已运行的开机服务会在切换版本前停止、随后恢复。首次安装开机服务执行 `tools/install-autostart.sh`。启动示例与服务诊断见 [README](../README.md)。

## 后台状态与卡片分页

目录自动按每页四张卡片分页，卡片整体点击打开；不需要应用自己创建“打开”按钮。`core.Snapshot.Running` 提供独立副本的运行状态映射，UI 在导航变化和定时刷新时同步右上角徽标。

应用若拥有由共享服务管理的独立后台工作，可以额外实现 `core.BackgroundReporter` 的 `BackgroundRunning() bool`，以线程安全、无阻塞方式上报。这个接口只提供状态，不修改 `Start/Stop` 的前台 context，也不启动后台任务；应用缓存不代表运行。默认离开即取消的生命周期继续适用于 PicoClaw 和设置。

主程序通过 `bootstrap` 注入共享外观偏好；设置应用通过 `theme.set` 保存选择。原生页面优先使用 `dock_button`、`dock_label`、`dock_panel` 的公共强调色样式和 `dock_theme_button`，以支持运行时主题变化。需要保持固有颜色的色板或状态图标应显式设置自己的颜色。

## 卡片图片图标

`core.Manifest.Icon` 支持单个字母或 LVGL 内置符号，例如设置齿轮 `"\uf013"`。`IconImage` 可选，填写相对于配置文件的本地 PNG 路径，例如 `"assets/app-icons/picoclaw.png"`。图片缺失或解码失败时保留 `Icon` 回退。

PNG 由 Go 标准库解码并转换为带透明度的 BGRA，输入限制 4 MiB、宽高各不超过 512 像素；C 侧复制像素数据并负责缓存和释放。显示时在 48×48 区域等比缩放，不随主题色重着色，图片区域仍由整张卡片处理点击。不需要在板端联网或启用额外 PNG 解码库。

内置图片放在 `src/assets/app-icons/` 并记录来源与许可；`bootstrap.sh` 复制到本机 `.local/assets/app-icons/`，部署脚本复制到板端 `assets/app-icons/`。PicoClaw 官方图标的来源、固定提交与校验值见该资源目录的 README。
