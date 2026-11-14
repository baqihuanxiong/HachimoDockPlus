# 按钮与摇杆实时测试

适用 Lyra Zero W / RK3506B、Linux 6.1.99、Buildroot。脚本 [input_probe.py](input_probe.py) 只用 Python 3 标准库，直接读取 GPIO 字符设备和 I²C ADC，无需安装 Python 包或修改固件。输出到终端的事件不注入 Linux `/dev/input`。

## 接线

依据 [2026-11-07 底板原理图](../../hardware/SCH_底板_2026-11-07.pdf) 第 1 页及 [引脚表](../../hardware/Pinout.md)。PI1 是核心板物理针号，U2 是底板排母针号。

| 输入 | 核心板 PI1 / 底板 U2 | 读取位置 |
|---|---|---|
| SW1 | 7 / 8 | GPIO0_A2，gpio0 offset 2 |
| SW2 | 11 / 12 | GPIO0_A3，gpio0 offset 3 |
| SW3 | 13 / 14 | GPIO0_A4，gpio0 offset 4 |
| 摇杆按键 JOYSTICK_KEY | 15 / 16 | GPIO0_A5，gpio0 offset 5 |
| 摇杆 X | U7 AIN0（pin 4） | ADS1015 单端通道 0 |
| 摇杆 Y | U7 AIN1（pin 5） | ADS1015 单端通道 1 |
| ADC SDA / SCL | 3、5 / 4、6 | GPIO0_A0/A1，I²C2 |

四个开关均为按下接地，需内部上拉。ADS1015 的 ADDR 接地，地址 `0x48`；AIN2/3 接地。摇杆和 ADC 供电为 3.3 V，ADC 使用 ±4.096 V 量程、1600 SPS 单次转换，12 位有符号结果右移 4 位后每计数为 2 mV。每次等待转换完成后读取，交替采样 AIN0/1。

## 运行

在项目根目录传到开发板，再通过 SSH 实时查看：

```sh
scp -O sandbox/keyboard/input_probe.py root@192.168.4.202:/tmp/input_probe.py
ssh -t root@192.168.4.202 'python3 /tmp/input_probe.py'
```

也可在板端直接运行 `python3 /tmp/input_probe.py`。需要 root 权限和 `/dev/gpiochip0`、`/dev/i2c-2`、`/dev/mem`；设备地址可用 `--i2c /dev/i2c-2 --address 0x48` 指定。启动时让摇杆自然居中，前约半秒采样中位数作为中心；若中心电压不在 0.7～2.6 V 内则退出报错。校准后再开始操作。

- 依次按下并释放 SW1、SW2、SW3、摇杆按键，检查对应的 `PRESS` / `RELEASE`；支持同时按下多个按钮。
- 摇杆沿两轴推到两端、斜推，再松手，检查 `JOYSTICK MOVE` 的原始值、电压、归一化坐标和方向，松手后应回到 `CENTER`。
- `X+` / `X-`、`Y+` / `Y-` 表示相对中心电压增减；实物安装方向需人工核对，未预设为上下左右。
- 每秒输出 `STATE`，其中按键 `1` 为按下、`0` 为释放。时间戳为脚本启动后的单调时钟秒数，不依赖板端系统时间。
- `Ctrl+C` 退出。正常退出、Ctrl+C、SIGTERM 或读取错误均会尝试恢复上拉寄存器原值、恢复 ADC 原配置并关闭 GPIO 句柄。

示例输出格式（事件示意）：

```text
[    0.510] SW1 INIT RELEASED
[    1.250] SW1 PRESS
[    1.460] SW1 RELEASE
[    2.100] JOYSTICK_KEY PRESS
[    2.800] JOYSTICK MOVE raw=(1450, 873) V=(2.900,1.746) xy=(+0.747,+0.000) dir=X+
[    3.200] JOYSTICK MOVE raw=( 861, 873) V=(1.722,1.746) xy=(+0.000,+0.000) dir=CENTER
```

默认每约 5 ms 轮询按键，稳定 20 ms 后报告事件；两轴每轮读取后间隔 20 ms，实际频率受 I²C 和系统调度影响，短于消抖时间的脉冲可能被忽略。摇杆使用 20% 方向死区和 5% 回差，方向变化或相对上次事件变化至少 12 计数（24 mV）时输出。归一化按实测中心及名义 0～3.3 V 计算并限制到 ±1，未校准端点。

```sh
# 连续输出每组 ADC 采样，便于检查微小变化
python3 /tmp/input_probe.py --raw

# 运行 10 秒后退出，或调整按键消抖和方向死区
python3 /tmp/input_probe.py --duration 10
python3 /tmp/input_probe.py --debounce-ms 30 --deadzone 0.25
```

## 当前内核的上拉处理

SDK 的 `drivers/gpio/gpio-rockchip.c` 中 `rockchip_gpio_set_config()` 仅处理消抖，GPIO ABI 的上拉请求实际未生效；初次实测 SW2、SW3、摇杆按键仍为下拉，出现静止误报按下。

脚本取得 gpio0 的四条输入线后，先核对运行设备树的 `rockchip,rk3506-ioc-pmu` 和寄存器范围，再通过 `/dev/mem` 临时设置 `IOC_PMU + 0x200`（`0xff950200`）中 GPIO0_A2～A5 的两位上拉字段。依据 SDK `pinctrl-rockchip.c` 的 `rk3506_calc_pull_reg_and_bit()`，编码 `01` 为上拉；写掩码为 `0x0ff0`，上拉值为 `0x0550`。保存并恢复的也仅是这四个字段，保留同一寄存器内 I²C2 的 A0/A1 及 A6/A7。

不支持该设备树或寄存器读回不符时退出，避免在其他芯片上访问固定地址。SIGKILL、断电无法执行清理；该设置不写入固件，重启恢复启动配置。后续正式适配应在设备树为按键配置 `bias-pull-up` 并注册输入驱动。

ADC 访问使用组合 I²C 事务，只访问指定地址，不扫描总线或发送广播复位；地址已被内核驱动占用时拒绝访问。运行时不要同时使用其他 ADC 读取脚本或 `i2cset` 操作 `0x48`，避免转换配置相互覆盖。GPIO 已被占用时脚本会报错退出。

## 验证记录

2026-11-14 在 `root@192.168.4.202` 上确认 Python 3.11.8、gpio0 offset 2～5 空闲、ADS1015 默认配置为 `0x8583`。补上临时上拉配置后完成 4 秒运行，四个开关静止时均为 `RELEASED`，摇杆中心原始值约 `(861, 873)`，即 `(1.722, 1.746) V`，稳定报告 `CENTER`。定时退出后上拉寄存器恢复为原值 `0x0000aa95`，ADC 配置恢复为 `0x8583`。

再次运行时，寄存器为 `0x0000a555`，pinctrl 确认 A2～A5 均为上拉；重复启动会以 `Device or resource busy` 拒绝访问。发送 SIGTERM 后正常退出，两处配置均恢复为上述原值。

主机检查通过 Python 语法编译及消抖、同时按键、释放、归一化、方向回差和 GPIO ABI 尺寸检查。完整机械操作仍需按上述步骤人工验收，不能仅凭静止采样判定所有按钮及摇杆行程正常。
