# 上游本地补丁

## LVGL 富文本行缓存

`lvgl-span-line-cache.patch` 对应固定的 LVGL v9.3.0 提交
`c033a98afddd65aaafeebea625382a94020fe4a7`。`bootstrap.sh` 和 `build.sh`
通过 `prepare-lvgl.sh` 幂等应用补丁。

原始富文本组件每次绘制都从头计算行位置。补丁在首次绘制时记录各行的
span、UTF-8 字节偏移及纵向坐标，后续通过二分查找跳过视口上方的文本。
文本、字体、样式或宽度变化时重建索引；销毁对象时释放索引。
索引来自实际绘制过程，保留原有跨 span 的换行、字体基线和 Markdown 样式。

`tests/picoclaw_scroll.c` 比较缓存与原始路径的完整像素，覆盖连续长段落、
多行富文本、宽度及主题变化，并检查滚动工作量不随文本总长度线性增长。
升级 LVGL 时需要重新核对补丁和这些测试。

## HachimoDock Windows ACL 幂等修复

[`hachimodock-windows-acl-idempotent.patch`](hachimodock-windows-acl-idempotent.patch)
对应 HachimoDock 子模块提交 `165401b3e2b1333022e8f0fdddcbebc14d669dba`，
从子模块工作区的三处本地修改原样导出。补丁及修改的上游内容沿用
[HachimoDock 项目许可证](../../HachimoDock/LICENSE)，不改授为本仓库的 MIT 许可。

部分 Windows 桌面进程可以读取已经符合要求的私有 ACL，但不能再次写入相同
ACL；原 `Protect-Entry` 每次调用都会执行 `Set-Acl`，导致这些已有目录或文件
无法使用。补丁先验证当前条目；所属用户及私有 ACL 已符合要求时直接返回，
不重复写权限。其他所有者仍被拒绝；不符合要求的 ACL 仍经过原修复与复验流程。

补丁同时修改两份 PowerShell 实现，并携带 Windows 回归测试：

- `integrations/dsh-hachimodock/windows-acl.ps1`
- `pc/src-tauri/bridge/packages/agent-session-bus/src/adapters/windows-acl.ps1`
- `pc/src-tauri/bridge/packages/agent-session-bus/test/local-capability.test.js`

测试先创建合法的私有目录和描述文件，再注入 `Set-Acl` 写入失败，验证
`prepare-directory`、`protect-file` 和 `verify-file` 仍可使用，且文件内容不变。

该补丁仅用于 Windows Desktop / DSH 集成，不参与本仓库的 Linux 板端应用构建，
不会自动修改子模块。从项目根目录手动应用：

```sh
git -C HachimoDock apply --check ../src/patches/hachimodock-windows-acl-idempotent.patch
git -C HachimoDock apply ../src/patches/hachimodock-windows-acl-idempotent.patch
```

应用后在子模块的 `pc/src-tauri/bridge/packages/agent-session-bus/` 目录执行：

```sh
node --test test/local-capability.test.js
```

完整 ACL 回归需在 Windows 上运行，Linux 会跳过 Windows 专属用例。
恢复干净的上游工作区时，从项目根目录执行：

```sh
git -C HachimoDock apply --reverse --check ../src/patches/hachimodock-windows-acl-idempotent.patch
git -C HachimoDock apply --reverse ../src/patches/hachimodock-windows-acl-idempotent.patch
```

升级子模块版本前先检查补丁是否仍需要、能否无冲突应用，再复测两份实现及回归用例。

2026-11-21 迁移验证：Linux 下该测试文件 7 项通过、3 项 Windows 专属用例跳过；
Windows 下新增的 ACL 禁写回归和目录联接拒绝用例通过。
既有“额外公共读取 ACE”用例在当前 Windows 环境的 ACL 修复步骤失败，
未修改的上游基线也在 ACL 准备步骤失败，因此本次不声明完整 Windows 套件通过。
日志保存在本地忽略目录 `src/build/reports/submodule-patch-2026-11-21/`。
