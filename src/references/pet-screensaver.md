# 桌宠屏保与 Desktop Wi-Fi

设置 → 外观选择“桌宠”，回到目录短按 SW3 进入；触摸与桌宠互动，短按 SW3 返回原页面。桌宠是公共屏保，不占组件位。亮屏使用期间远端状态不切换页面；关闭显示待机期间，新的有效任务可自动亮屏并进入桌宠屏保，不依赖所选屏保模式。离开屏保或关闭显示后回收视频解码进程，Wi-Fi 接收服务继续运行。动画静音，不接管音乐或语音设备。自动待机时长、长按 SW3 及唤醒规则见[设置说明](settings.md#外观与状态栏)。

顶部状态和下方 Agent 标题、正文统一使用 22px 字体，连接状态显示“已连接”或“未连接”（10 秒未收到探测或状态即断开）；配对细节在设置 → 系统页底部查看。右上角仅显示时分（`HH:MM`），使用 `ui.timezone`，未对时时显示“等待对时”。

触摸遵循官方动作选择：左侧三分之一区域优先 `touch.left`，右侧三分之一区域优先 `touch.right`；没有方向素材或点击中部时轮换 `touch.lick` / `touch.what`，缺少其中一段则使用另一段。互动单次播放完整视频，最短保持 1.2 秒，然后恢复待机；播放中重复点击不重启动画。没有触摸素材时保持原动画。Agent 正在工作、等待确认或显示完成 / 错误时优先展示任务，暂不触发互动；互动中收到任务状态立即切回任务动画。

## 离线导入

从官方客户端生成的形象目录取出 `videos/*.mp4`，也支持平铺的 `<动作>.mp4`。工具只取状态视频，不复制 API 配置、原始照片或上位机私有数据；不依赖原 manifest 的绝对 `videoPath`。本版不实现 USB / Wi-Fi 素材传输，不加载 P4 `.h264` 裸流包。

在开发主机的 `src/` 下执行：

```sh
# 已有开发镜像需重建一次以加入 FFmpeg。
docker build -t hachimodock-go-lvgl:dev -f tools/Dockerfile .
python3 tools/import-pet.py /path/to/exported-appearance \
  --name 我的宠物 --output build/my-pet

# 使用当前上游子模块的西高地测试；输出目录必须尚不存在。
python3 tools/import-pet.py --builtin --output build/westie
```

主机没有 FFmpeg 时自动使用该 Docker 镜像。默认归一化为 640×480、15 fps、H.264 Baseline、等比黑底静音 MP4；板端无需编码器。`--copy` 可直接复制已归一化素材。单视频不超过 16 MiB、工具整包不超过 24 MiB；SMB 全部 assets 仍共用原有 32 MiB 总额，须为字体等资源留空间。

将输出目录的**内容**通过 SMB 上传到以下位置，并在共享 config.json 的原有 `ui` 中合并配置：

```text
HachimoConfig/assets/pets/my-pet/
├── manifest.json
└── videos/
    ├── idle.default.mp4
    ├── working.mp4
    ├── waiting_user.mp4
    ├── done.mp4
    └── error.mp4
```

```json
"pet": {"directory": "assets/pets/my-pet", "wifi_enabled": true}
```

保存后执行设置 → 系统 → 应用配置并重启。沿用 SMB 快照校验与原子切换；上传中的候选目录不会直接播放，清单损坏、链接文件和重复状态会拒绝应用。实际视频解码失败在屏保显示错误。未配置时目录默认 `assets/pets/default`，Wi-Fi 默认关闭。

支持 `working.thinking.mp4` 等工作变体和各种 `idle.*`。缺失某状态回退到已有待机素材；无任何视频时显示导入提示。可选 manifest 只读取 `name` / `appearanceName` 作为显示名，视频按文件名发现，不跟随清单中的桌面绝对路径。

## 官方 Desktop 状态跟随

电脑与设备处于同一受信任局域网。启用 Wi-Fi 配置并重启后，在官方 Desktop 的 Wi-Fi 设备配对界面扫描、选择未配对设备确认。状态跟随无需 USB 授权。已有配对不会被另一台电脑覆盖；换电脑前在设备“设置 → 系统 → Desktop Wi-Fi 配对 → 解除配对”，并清除旧电脑侧记录。

实现基于官方提交 `165401b`：UDP `29462` 发现、TCP `29461`，SRP-3072 / SHA-512 自动首次配对，HMAC-SHA256 派生、每连接随机数及 AES-128-GCM 封装。仅接受同子网来源。私有配对记录为 `data_dir/pet/lan.json`，权限 600，不进入 SMB；启动 ID 每次启动更新。官方扫描器要求 `p4-` 加 12 位十六进制身份，因此沿用这个**线协议 ID**；固件名如实报告 `linux-go-lvgl-0.1.0`。

本版只开放发现、自动配对、探测、状态查询和展示快照（模式 0 / 2 / 5 / 6）。不声明有线信任、远程控制、组件、固件升级、麦克风或 USB。Desktop 对这些其他功能显示的 USB 授权提示不影响状态跟随。

选中会话决定动画及任务文字，优先于 Agent 汇总状态。相同终态修订不延长完成倒计时，旧修订不能覆盖新状态。快照超过 10 秒清除旧任务并回待机，独立探测不会续期任务；待机每 20 秒轮换已有 idle 素材。

## 验证

```sh
tools/dev.sh sh -c 'go test -race -p 2 -tags stdjson,goolm ./... && go vet -tags stdjson,goolm ./...'
tools/dev.sh tools/build.sh native
tools/dev.sh tools/check-ui.sh
tools/dev.sh tools/build.sh arm

# 偏好文件中已选桌宠；预览不启动网络、不申请物理输入。
tools/dev.sh build/native/dock -config .local/config.json \
  -page screensaver -preview build/pet.png
```

`-page screensaver` 也支持实机启动。独占显示、触摸测试前停已有服务，结束恢复。播放器通过板端 FFmpeg 软件解码和有界帧缓存交给 LVGL，未接入 MPP；归一化素材的性能仍需结合实际录放音负载验收。

独立协议客户端 [desktop_lan_probe.py](examples/desktop_lan_probe.py) 需要主机 Python `cryptography`：

```sh
python3 references/examples/desktop_lan_probe.py --host 开发板IP \
  --key-file build/pet-probe.key --pair --state working --seconds 10 --tamper
```

命令会真实配对未配置设备，Key 文件不应公开。后续去掉 `--pair`，使用同一文件测试 idle / working / waiting_user / done / error。验证后在设备解除测试配对，再连接正式 Desktop。此脚本检验线协议，不等同于官方 GUI 和真实 Agent 的端到端验收；离屏预览、实机帧缓冲和物理触摸也分别记录。

西高地素材和派生协议保留上游非商业许可；内置包保留来源、提交和 LICENSE。见[第三方说明](../../THIRD_PARTY_NOTICES.md)。
