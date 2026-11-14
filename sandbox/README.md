# 开发试验

按功能存放开发阶段的验证脚本、试验程序和排查记录，并在各目录说明使用的开发板、固件基线、执行方法及结果。

| 目录 | 内容 |
|---|---|
| [display](display/README.md) | Lyra Zero W 的 WLK2802MIPI 屏幕与 CST826 触摸适配历史 |
| [audio](audio/README.md) | Lyra Zero W 的 CM108B USB 音频检查与后续试验 |

可复用的设备树、内核配置、驱动补丁及固件工具整理到 `board/<开发板名>/`；应用代码放在 `src/`。SDK、分区备份与固件产物继续放在被 Git 忽略的 `sdk/` 中，试验记录可引用其路径和校验值。
