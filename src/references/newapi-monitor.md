# NewAPI 网关监控

组件 ID 为 `newapi-monitor`，在组件中心打开。仅进入页面时刷新，离开后取消前台请求、自动刷新和重置倒计时，再次进入重新读取数据。页面为两列单页看板：左上显示今日／本周／本月的消耗、调用数与总体成功率，右上显示上游余额，下方分别为 Top 模型调用占比和 Top 用户消耗柱状图。榜单卡片可独立上下滑动，保留阅读位置，支持白天 / 黑夜模式。

## 配置

将下面字段合并到现有 `apps`，不要覆盖其他应用配置。相对路径以 `config.json` 所在目录为基准。地址填 NewAPI 部署根地址，不要追加 `/v1`。

```json
{
  "newapi-monitor": {
    "url": "https://newapi.example.com",
    "key_file": "secrets/newapi-admin.key",
    "admin_id": 1,
    "top_n": 10,
    "reset_usd": 200,
    "refresh_seconds": 300,
    "timeout_seconds": 20,
    "max_logs": 50000,
    "timezone": "Asia/Shanghai",
    "providers": [
      {
        "kind": "aliyun-token-plan",
        "name": "阿里千问 Token Plan",
        "auth": "access-key",
        "url": "https://bailian-cs.console.aliyun.com",
        "key_file": "secrets/aliyun-ram.key",
        "region": "cn-beijing"
      },
      {
        "kind": "mimo-token-plan",
        "name": "小米 MiMo Token Plan",
        "url": "https://token-plan-cn.xiaomimimo.com/v1/platform/token_plan",
        "key_file": "secrets/mimo.key"
      }
    ]
  }
}
```

除 `url` / `key_file` 外的主配置字段均可省略，使用上述默认值；`providers` 默认空数组，未配置时上游余额页显示配置提示。`timezone` 默认继承 `ui.timezone`。所有密钥文件必须是权限 `600` 的普通文件，凭证不进入状态快照、日志、截图或版本控制。

`admin_id` 必须匹配管理员账号 ID。`newapi-admin.key` 使用 NewAPI 个人设置生成的**管理员访问令牌 / Access Token**，模型调用所用的 `sk-` API Key 无法访问管理接口。查询采用 Bearer 和 `New-Api-User` 请求头。

管理员令牌在应用实例首次创建时读取；更换该文件后请应用配置并重启程序。提供商凭证在每次刷新时读取。

`top_n` 支持 1～100；刷新间隔至少 30 秒，单次 HTTP 超时 1～120 秒。一次完整刷新最多 2 分钟，批量重置最多 10 分钟。设置应用的“应用配置并重启”会先做配置和密钥文件校验，不访问网络。

通过 SMB 编辑时，使用共享的待应用配置及 `secrets/`，再应用并重启；后续部署不会自动覆盖板端配置或安装新密钥。详见 [配置共享](configuration-sharing.md)。

## 统计与排行

- 今日从配置时区当天零点开始；本周从周一零点开始；本月从每月1日零点开始，均截至本次刷新开始时刻。只扫描一次所需范围的日志，分别聚合三个指标，不将七日数据充当月度数据。
- 美元消耗使用 `/api/status` 的 `quota_per_unit` 换算。调用数与成功率按日志 `type=2`（成功消耗）和 `type=5`（错误）计算；无样本显示 `--`，未记录的错误和内部重试不额外推算。
- 下方两榜均为最近7个自然日。模型按调用数（含已记录失败）排序，柱条表示占全部调用的比例；用户按成功消耗美元金额排序，柱条相对于榜首等比绘制。两榜共用 `top_n`（1～100），默认10，首屏每列可见5项，其余在卡片内滑动查看。
- 上游余额显示账户名称、精简余额数字和单位／剩余比例，最多显示配置中的前2个账户。不再展示套餐代码、长时间字段或完整接口信息。
- 不再查询渠道列表或展示渠道／单模型成功率。旧配置中的 `channel_ids`、`healthy_percent` 仅为兼容保留，已不参与看板。
- 日志以100条分页，边读边聚合；达到 `max_logs` 时明确提示统计不完整。分页重复、数量变化或缺失时刷新失败，保留上一份数据。上游账户查询失败仅影响该账户。

## 阿里千问 Token Plan 令牌

当前接入百炼个人版 Token Plan 月度额度，指标卡显示剩余百分比；该接口未返回绝对 Credits 时不会编造金额。套餐额度与阿里云现金余额相互独立。

### RAM 自动获取与续期（板端可独立运行）

阿里账户配置增加 `"auth": "access-key"`，`key_file` 改为 `secrets/aliyun-ram.key`，URL 与地域保持对应的官方 HTTPS 地址。例如：

```json
{"kind":"aliyun-token-plan","name":"阿里千问 Token Plan","auth":"access-key","url":"https://bailian-cs.console.aliyun.com","key_file":"secrets/aliyun-ram.key","region":"cn-beijing"}
```

`aliyun-ram.key` 是权限 `600` 的**单行 JSON**（示例值仅作占位）：

```json
{"access_key_id":"LTAI_REPLACE_ME","access_key_secret":"REPLACE_ME"}
```

可选 `security_token` 支持临时 STS 凭证，但上层仍须在 STS 到期时更换凭证文件。配置共享应用时会校验 RAM JSON 和文件权限，不发起网络请求。

在应用前台首次查询时，板端按 ACS3-HMAC-SHA256 签名调用 `POST https://modelstudio.<region>.aliyuncs.com/modelstudio/cli/generateAccessToken`（`GenerateCLIAccessToken`，版本 `2026-02-10`），再查询额度。令牌只保存在本进程内存；之后的刷新复用缓存。HTTP 401 或 `BailianGateway.Login.NotLogined` 触发重新获取并重试额度请求；每次刷新最多换取一次令牌。权限、网络和格式错误不会反复续期。并发刷新共用令牌，等待和网络请求均服从页面取消；离开页面后不运行后台续期任务，再次进入按需恢复。应用重启后重新取令牌，不依赖电脑、浏览器或 CLI 常驻。

RAM 密钥文件每次刷新读取，替换后下一次刷新废弃旧令牌。密钥和令牌不进入状态、日志或截图；错误只显示固定的权限／鉴权／网络提示。RAM 凭证本身仍需保持有效，撤销或停用后无法续期。

RAM 用户授权包含 `AliyunTokenPlanReadOnlyAccess`、`AliyunBSSReadOnlyAccess`，以及允许 `modelstudio:GenerateCLIAccessToken`、`Resource: "*"` 的自定义策略。百炼账号管理还须将用户加入对应业务空间；北京个人版当前查询接口需要“应用 → 智能体-操作”权限。权限管理是独立于 RAM 系统策略的另一层。

依据：[官方令牌获取实现](https://github.com/modelstudioai/cli/blob/main/packages/core/src/auth/refresh-token.ts)、[ACS3 签名规范](https://help.aliyun.com/zh/sdk/product-overview/v3-request-structure-and-signature)、[Token Plan RAM 权限说明](https://help.aliyun.com/zh/model-studio/token-plan-permissions-and-usage)。

### 手工控制台令牌（兼容原有配置）

省略 `auth` 时继续将 `key_file` 作为控制台令牌文件读取，不自动换取令牌：

1. 按 [官方安装说明](https://help.aliyun.com/en/model-studio/cli/installation) 安装百炼 CLI。
2. 在电脑执行 `bl auth login --console --config default`，在浏览器完成阿里云控制台登录与授权。浏览器登录不接受 `--console-region`；该参数仅用于 `--console --token-stdin` 导入已有令牌。
3. 执行 `bl usage token-plan --config default --output json` 确认该账号存在月度额度。
4. 将 CLI 配置的 `access_token` 单独写入 `aliyun-console.key`，权限设为 `600`，再通过配置共享安装到设备。以下示例从项目 `src/` 运行，不在终端输出令牌：

```sh
python3 - <<'PY'
import json, os
from pathlib import Path
base = Path(os.environ.get("BAILIAN_CONFIG_DIR", str(Path.home() / ".bailian")))
token = json.loads((base / "config.json").read_text())["access_token"]
target = Path(".local/secrets/aliyun-console.key")
target.parent.mkdir(parents=True, exist_ok=True, mode=0o700)
fd = os.open(target, os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 0o600)
with os.fdopen(fd, "w") as stream:
    stream.write(token.strip() + "\n")
target.chmod(0o600)
PY
```

海外账号按 CLI 的 `--console-site international` 和实际地域授权。新加坡国际控制台使用 `region: ap-southeast-1` 及 `url: https://bailian-singapore-cs.alibabacloud.com`；国内站新加坡网关为 `https://modelstudio-cs.console.aliyun.com`。控制台令牌过期后重新授权、更新文件并重新进入应用。

实现依据为 [官方 CLI 的认证说明](https://help.aliyun.com/en/model-studio/cli/config)、[用量说明](https://help.aliyun.com/zh/model-studio/cli/usage-quota) 和 [官方网关实现](https://github.com/modelstudioai/cli/blob/main/packages/core/src/console/gateway.ts)。请求 `/cli/api.json` 的 `zeldaHttp.apikeyMgr./tokenplan/personal/api/v2/usage`，解析多层 `data.DataV2.data.data`；不使用模型 Key 替代控制台令牌。

## 小米 MiMo Token Plan

直接使用 Token Plan API Key，写入 `mimo.key`，不需要 Cookie。`url` 为**完整查询地址**：

```text
GET https://token-plan-cn.xiaomimimo.com/v1/platform/token_plan
api-key: <Token Plan API Key>
```

已于 2026-11-15 使用本地凭证实测该接口，同时验证 `api-key` 和 Bearer 均可鉴权；应用采用 `api-key`。解析 `object: token_plan`、`quota.total_credits` / `used_credits` / `remaining_credits`、`plan_code`、`period.end_at`，指标卡显示精简剩余 Credits 和剩余比例（k/M/B 分别表示千／百万／十亿），完整额度和周期数据仍保留在业务结果中，不把 Credits 当作美元。接口失败或格式变化显示错误，不回退为零余额。

## 全用户余额重置

点击“重置余额”进入 **10 秒执行倒计时**，弹层显示本次每用户美元额度。

| 输入 | 行为 |
|---|---|
| 倒计时归零 | 自动执行 |
| SW2 按下 / 立即执行按钮 | 提前执行一次，后续重复确认不重复提交 |
| SW3 按下 / 取消按钮 | 返回监控页，取消倒计时，不修改余额 |
| 离开页面 / 退出进程 | 取消倒计时与后续网络任务，重入不恢复倒计时 |
| 已开始写入后 SW3 返回目录 | 停止后续用户处理；已发送请求可能生效，需核对部分结果 |

执行前完整分页获取所有未删除用户（含停用用户及管理员），检查操作者能否管理全部角色；root 可管理自身及其他 root。权限不足或分页不完整时不写入。逐用户发送：

```json
{"id": 123, "action": "add_quota", "mode": "override", "value": 100000000}
```

接口为 `POST /api/user/manage`。示例对应 `quota_per_unit=500000` 时的 $200；实际从服务读取换算。每个请求成功后通过 `GET /api/user/:id` 回读核对余额，显示已核对数量。失败、超时、余额不符时停止后续处理并报告部分结果，不自动重试，不将成功响应包等同余额变更。并发调用会影响回读余额，应在低流量时使用。此功能将余额**设为目标值**，不清除历史消耗、用户记录或 API Token 配额；批量操作不是跨用户事务。

协议依据为 [NewAPI rc.40 管理接口](https://github.com/QuantumNous/new-api/blob/v1.0.0-rc.40/controller/user_quota.go)。不兼容旧接口时显示失败，不使用编辑用户资料的接口尝试写余额。

## 验证与终端

```sh
tools/dev.sh sh -c 'go test -race -p 2 -tags stdjson,goolm ./... && go vet -tags stdjson,goolm ./...'
tools/dev.sh tools/build.sh native
tools/dev.sh tools/check-ui.sh
tools/dev.sh sh -c 'NEWAPI_MONITOR_CONFIG=/work/.local/config.json go test -tags stdjson,goolm -run TestLiveReadOnly -v ./internal/apps/newapi'
tools/dev.sh build/native/dock -config .local/config.json -preview build/newapi.png -page newapi-monitor
tools/dev.sh tools/build.sh arm
```

`TestLiveReadOnly` 只查询，默认跳过，绝不调用重置接口。原生 C 检查验证指标卡、两列榜单、独立滚动、数据更新保留位置、双主题、倒计时和重复卸载。CLI `-ui terminal -page newapi-monitor` 支持 `/refresh`、`/reset`、`/confirm`、`/cancel`。


板端重置回归可使用 `python3 tests/newapi_fixture.py --state build/fixture-state.json`，该服务仅绑定主机回环地址，含 3 个合成用户。通过 `ssh -R 18081:127.0.0.1:18081` 转发到板端，在 `/tmp` 的独立配置中把 NewAPI 地址设为 `http://127.0.0.1:18081`、`data_dir` 指向临时目录、字体指向已安装的绝对路径，暂停正式 UI 后启动同一个板端二进制。分别验证取消产生 0 次写入、立即执行产生 3 次写入、倒计时归零再产生 3 次写入，以及各用户 `quota=100000000`；结束后停止临时程序和转发，清理临时配置并恢复原服务。**重置验收不得将此夹具替换为真实网关或代理真实写入。**

应用图标使用 NewAPI 官方仓库的原始透明 PNG，来源、固定提交、校验值和许可见[应用图标来源](../assets/app-icons/README.md)。沿用项目共享字体。此应用只需要网络，缺少显示设备时可使用终端诊断，不申请音频、麦克风或额外传感器。验证结论见 [单页看板验证记录](reports/newapi-dashboard-2026-11-15.md)。
