# 米家属性读取修复实机验证

2026-11-15 23:45（Asia/Shanghai），开发板 `root@192.168.5.156`，ARMv7 / Linux 6.1.99。

## 部署

- 部署独立米家 CLI 和配套技能、README、许可文件，未替换主应用二进制。
- 生效配置 `/opt/hachimodock/agent`、待应用区 `/opt/hachimodock/config-share/agent`、运行工作区 `/opt/hachimodock/data/workspace` 的扩展文件同步更新，避免后续应用配置时回退。
- 三处 CLI 的 SHA-256 均为 `e6bf46dfec59ea0dbfeef5b697bddafe6c33474038d709c211bcaca0823142a3`，与本机 ARMv7 产物一致。
- 三处技能的 SHA-256 均为 `738022ebb7b983efcf43d7a39de5bb2d12337ad24ad6f9c0b3ce4d1d5db1419a`，与源码一致。
- 短暂停止并重新启动应用加载扩展；验证时 supervisor PID 为 2401，实际应用 PID 为 2406，SMB 服务正常。
- 原有授权继续有效，未读取或输出令牌内容；临时部署包及回滚副本已清理。

## 真实云端读取

目标为“弱电柜插座”，型号 `cuco.plug.v3`，规格为 `urn:miot-spec-v2:device:outlet:0000A002:cuco-v3:2`。

| 验证项 | 结果 |
| --- | --- |
| 旧版 `device get DID` | 退出码 1，错误 `-704220043`，未返回其他成功读数 |
| 新版 `device get DID 11 2` | 退出码 0，`code: 0`、`value: 110` |
| 新版 `device get DID` | 退出码 0，33 项结果中 32 项成功、1 项失败；功率读数仍为 110 |
| 失败属性定位 | SIID=1、PIID=5，Device Information / Serial Number；错误 `-704220043`，保留错误说明且不返回 value |
| 功率单位 | 官方 MIOT 实例定义 SIID=11、PIID=2 为 Electric Power，`unit: watt`，注释“电功率（单位：1W）”；此次读数为 **110 W** |

此次故障由序列号属性读取失败触发旧版整批失败；功率属性本身读取正常。验证使用原有授权和设备缓存，通过正式运行路径中的 CLI 连接真实小米云端，未执行属性写入、动作或场景。

主机 `go test -race ./...`、`go vet ./...` 通过，ARMv7 静态二进制构建通过。此记录验证 CLI 的真实读取和部署状态，未执行界面或自然语言对话端到端验收，也未验证设备控制。
