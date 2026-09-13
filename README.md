# OpenWrt 全栈流量监控系统

高可靠、低 CPU 开销的集中式流量监控：OpenWrt Agent → 公网 Gin API → 单页监控大屏。

```
OpenWrt Agent (Go)  --POST /api/v1/traffic/report-->  Gin Server (SQLite 日表 + VictoriaMetrics 时序)
                                                          |
Web Dashboard (traffic.html) <--GET /api/v1/traffic/stats--+
                              <--GET /api/v1/traffic/history-+
```

## 目录结构

```
flow-statistics/                 # 本仓库：Agent + 安装脚本 + 文档
├── agent/                       # 路由器端 Daemon（可静态编译单文件）
│   ├── main.go
│   ├── collector.go             # Wi-Fi station 累计计数差分
│   ├── wifi.go                  # iw station dump 解析
│   ├── meta.go                  # dhcp.leases / ARP 主机名与 IP
│   ├── queue.go                 # 离线 FIFO
│   └── client.go                # HTTP 上报 + 指数退避
└── scripts/                     # 安装/卸载/冒烟脚本

# 业务后端与大屏在兄弟目录（非本仓库子目录）：
../server_all/
├── service/traffic/             # Gin 子模块：vm.go + sqlite.go + store.go
├── app/traffic.html             # 监控大屏（部署到站点 /app/traffic.html）
└── service_all.service          # systemd 模板
```

Agent **只读**采集（`iw` / leases / ARP），不写 uci/nvram/iptables。

## 1. Server 端接入

### 生产：已并入 `server_all`

路径：`../server_all/service/traffic/`，在 `server_all/main.go` 注册。

| 路由 | 鉴权 |
|------|------|
| `POST /api/v1/traffic/report` | **免网关 Cookie**，校验 `X-Device-Token` |
| `POST /api/v1/traffic/reset` | 同上 |
| `GET /api/v1/traffic/stats` | 需网关登录；**强制** `router_id` |
| `GET /api/v1/traffic/history` | 需网关登录；**强制** `router_id`；最长 **31 天** |
| `GET /api/v1/traffic_device` | 需网关登录；后门列出已记录 `router_id`（**前端勿请求**） |

网关放行：`authentication/openresty/conf.d/auth_gateway.conf`  
大屏静态页：`server_all/app/traffic.html` → 网关 `/app/traffic.html`

存储拆分：

| 数据 | 后端 |
|------|------|
| 时序（速率/历史曲线） | **VictoriaMetrics**（Influx write + PromQL `query_range`） |
| 设备元数据 + 日累计 | **SQLite** 表 `daily_traffic`（保留 31 天，每日 04:00 purge） |

旧 `rate_samples*` 已废弃（启动 DROP，不迁历史）。

```bash
cd ../server_all
# 设置 TRAFFIC_DEVICE_TOKEN / TRAFFIC_DB_PATH / TRAFFIC_VM_URL 后
go build -o service_all .
# 或 Windows：build.bat
```

环境变量：

| 变量 | 说明 | 默认 |
|------|------|------|
| `TRAFFIC_DEVICE_TOKEN` | Agent 上报 Token | `change-me-traffic-token` |
| `TRAFFIC_DB_PATH` | SQLite 路径 | `data/traffic.db` |
| `TRAFFIC_VM_URL` | VictoriaMetrics 基址；`off`/`none` 关闭 | `http://127.0.0.1:8428` |

### API

**POST `/api/v1/traffic/report`**

- Header: `X-Device-Token: <SECRET>`
- Body: 单个对象或数组（离线批量补发）

**GET `/api/v1/traffic/stats?router_id=<required>`**

- **`router_id` 必填**，缺省返回 `400 router_id required`
- 返回该路由的速率、在线数、设备列表（含今日累计）
- 大屏：输入框为空时**不发请求**；回车后才查询并写入本地历史

**GET `/api/v1/traffic/history`**

- **`router_id` 必填**
- 窗口：`hours`（默认 24，最长 31×24），或 `from`/`to`（unix），或 `date_from`/`date_to`（`YYYY-MM-DD`，本地日）
- 时序来自 VictoriaMetrics；响应含 `bucket_sec`、`from`/`to`、`hours`
- 颗粒度随跨度：≤6h→60s，≤48h→120s，≤7d→5m（>3d 用 15m），>7d→1h
- 前端控件：`historyRange` / `dateFrom` / `dateTo`（非旧的 `historyHours`）

**GET `/api/v1/traffic_device`**（运维后门，**前端勿请求**）

- 返回已记录过的全部 `router_id`
- Agent 侧 `router_id`：环境变量 `TRAFFIC_ROUTER_ID` 或 `-router-id`（默认 `main-router-01`）

## 2. 路由器 Agent

### 一键安装（推荐，仅 arm64 + 内核 ≥ 6.12）

发布目录：`https://dagongren.tech/public/flow-statistics/`

| 文件 | 说明 |
|------|------|
| `traffic-agent-linux-arm64` | 交叉编译静态二进制 |
| `flow-statistics_install.sh` | 安装：下载、随机 ROUTER_ID、procd 服务 |
| `flow-statistics_uninstall.sh` | 卸载（默认保留 `/etc/flow-statistics`） |

```bash
# 安装（结束时打印 ROUTER_ID）
wget -qO- https://dagongren.tech/public/flow-statistics/flow-statistics_install.sh | sh

# 卸载（保留配置与 ROUTER_ID，重装可复用）
wget -qO- https://dagongren.tech/public/flow-statistics/flow-statistics_uninstall.sh | sh

# 卸载并删除配置
FS_PURGE_CONF=1 sh flow-statistics_uninstall.sh
```

安装脚本行为：

- 校验 `uname -m` 为 `aarch64`/`arm64`，内核主版本 ≥ `6.12`
- 需要 `iw` + `curl|wget|uclient-fetch`
- **ROUTER_ID**：若 `/etc/flow-statistics/agent.conf` 已有则复用；否则随机 10 位字母数字；可用 `FS_ROUTER_ID=xxx` 指定
- 二进制：`/usr/sbin/traffic-agent`，服务：`/etc/init.d/flow-statistics`
- 默认上报：`https://dagongren.tech/api/v1/traffic/report`

覆盖示例：

```bash
FS_ROUTER_ID=home-ax6000 FS_INTERVAL=30s sh flow-statistics_install.sh
```

### 本地交叉编译

```bash
cd agent
# OpenWrt aarch64（当前正式发布架构）
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="-s -w" -o traffic-agent-linux-arm64 .

# 可选：mipsle softfloat
GOOS=linux GOARCH=mipsle GOMIPS=softfloat go build -ldflags="-s -w" -o traffic-agent .
```

手动运行：

```bash
./traffic-agent \
  -server https://your.domain/api/v1/traffic/report \
  -token your-secret \
  -router-id main-router-01 \
  -interval 30s
# 可选：固定 AP 接口，默认自动发现
#  -wifi-ifaces phy0-ap0,phy1-ap0
```

| 参数 / 环境变量 | 说明 |
|-----------------|------|
| `-server` / `TRAFFIC_SERVER` | 上报 URL |
| `-token` / `TRAFFIC_TOKEN` | 与 Server 一致的 Token |
| `-router-id` / `TRAFFIC_ROUTER_ID` | 路由器 ID |
| `-interval` / `TRAFFIC_INTERVAL` | 采样窗口，默认 **30s** |
| `-wifi-ifaces` / `TRAFFIC_WIFI_IFACES` | AP 接口列表，逗号分隔；空=自动发现 |
| `-queue-max` | 离线 FIFO 上限（默认 120 ≈ 1h@30s） |

### 采集逻辑

1. **数据源**：`iw dev <ap> station dump` 的驱动级累计 `rx bytes` / `tx bytes`（**仅 Wi‑Fi 已关联设备**）。
2. **方向映射**（AP 视角 → 客户端）：`iw tx` = 下行（下载），`iw rx` = 上行（上传）。
3. **差分**：按 **MAC** 对累计计数器做 `cur - prev`；新出现站点本周期只建基线，避免把历史总量算进今日。
4. **元数据**：`/tmp/dhcp.leases`、`/proc/net/arp` 补全 IP / 主机名。
5. **可靠性**：上报失败入内存 FIFO，指数退避后批量补发。

> 不用 PPE / conntrack / iptables / DPI。CPU 开销低。  
> **范围**：有线客户端不统计。路由器需有 `iw`。

## 3. Web 大屏

生产页面：`server_all/app/traffic.html`，经网关托管为 `/app/traffic.html`（需登录）。

改完后请：

1. 保留本地 `C:\Documents\useful\go\server_all\app\traffic.html`
2. 部署到 hkserver：`/www/wwwroot/dagongren.tech/app/traffic.html`（`www:www`）

页面能力概要：

- 实时：总上下行、在线数、设备表与今日累计
- 历史：预设 6h–31d + **自选日期**（控件 `historyRange` / `dateFrom` / `dateTo`）
- 图表依赖页面内 ECharts / Chart 相关静态资源

## 协议字段约定

Agent 上报的 `rx_bytes` / `tx_bytes` 为**本统计窗口增量**（由 Wi‑Fi station 累计计数差分得到）。

Server 用 `bytes / interval_sec` 得到 B/s（展示用），累加当日 `today_rx` / `today_tx`（SQLite），并把速率点写入 VictoriaMetrics。

部署新 agent 后建议清一次脏数据：

```bash
curl -X POST -H "X-Device-Token: $TOKEN" "https://your.domain/api/v1/traffic/reset"
```
