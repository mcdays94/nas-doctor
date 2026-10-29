[English](README.md) | **简体中文**

> **翻译说明**：本文是[英文 README](README.md) 的简体中文译文，英文版是权威来源；如有差异，请以英文版为准。最后同步的英文原文提交：[2c3d551](https://github.com/mcdays94/nas-doctor/blob/2c3d551eaa88486751f35ea38ceb95d8d3f5a14a/README.md)。

<p align="center">
  <img src="icons/icon3.png" alt="NAS Doctor" width="128" height="128">
</p>

<h1 align="center">NAS Doctor</h1>

<p align="center">
  <strong><em>你安心入睡，服务器彻夜不眠。</em></strong>
</p>

<p align="center">
  <strong>本地 NAS 诊断与监控工具。</strong><br>
  以 Docker 容器的形式运行在 Unraid、TrueNAS、群晖、Proxmox 或 Kubernetes 集群上。<br>
  提供美观的仪表盘、Prometheus 指标和 Webhook 告警功能，无需云端账号。<br>
</p>

> **Beta（测试版）** — NAS Doctor 正在积极开发中。核心功能已在 Unraid 上经过测试，运行稳定。其他平台在某些边缘场景下仍可能出现问题。[在此反馈问题。](https://github.com/mcdays94/nas-doctor/issues)

<p align="center">
  <a href="https://github.com/mcdays94/nas-doctor/pkgs/container/nas-doctor"><img src="https://img.shields.io/endpoint?url=https%3A%2F%2Fnas-doctor-stats.lusostreams.workers.dev%2Fbadge-monthly.json&style=flat-square&logo=docker&logoColor=white" alt="GHCR pulls/month"></a>
  <a href="https://buymeacoffee.com/miguelcaetanodias"><img src="https://img.shields.io/badge/Buy%20Me%20A%20Coffee-support-yellow.svg?style=flat-square&logo=buy-me-a-coffee" alt="Buy Me A Coffee"></a>
</p>

---

![NAS Doctor 仪表盘](screenshots/midnight-top.jpg)

NAS Doctor 定期为服务器进行健康检查，分析 SMART 数据、磁盘使用情况、Docker 容器、GPU、网络速度、进程 CPU 使用率、内核日志、温度、ZFS 存储池、UPS 电源和 Unraid 奇偶校验情况。它会为诊断结果标明严重程度，通过关联分析定位根本原因，并提供可执行的建议（依据 Backblaze 硬盘故障率数据）。

---

<a id="table-of-contents"></a>

## 目录

- [功能介绍](#what-it-does)
- [快速开始](#quick-start)
  - [Docker Compose](#docker-compose-recommended)
  - [Unraid](#unraid--docker-ui-setup)
  - [群晖 DSM](#synology-dsm--container-manager)
  - [TrueNAS SCALE](#truenas-scale)
  - [Kubernetes](#kubernetes-k3s--k8s)
  - [Proxmox](#proxmox-via-ubuntu-vm--lxc)
  - [从源码构建](#build-from-source)
- [截图](#screenshots)
- [设置](#settings)
- [API 参考](#api-reference)
- [Prometheus 指标](#prometheus-metrics)
- [支持的平台](#supported-platforms)
- [文件结构与数据位置](#file-structure--data-locations)
- [资源占用](#resource-usage)
- [诊断报告](#diagnostic-report)

---

<a id="what-it-does"></a>

## 功能介绍

<a id="diagnostics"></a>

### 诊断

- **SMART 健康状态**：查看每块硬盘的健康状态、温度、已重映射扇区、待重映射扇区、UDMA CRC 错误、通电时长和 ATA 端口映射，并采用 **Backblaze 故障率阈值**（2025 年第四季度数据，超过 33.7 万块硬盘）。默认情况下，NAS Doctor 会尊重硬盘待机状态，跳过已停转的硬盘，而不会为了读取 SMART 数据将其唤醒——对于会停转的硬盘，历史记录中会出现空缺，这是有意为之（可减少磨损）。在 Settings → Advanced（设置 → 高级）中开启 **Wake drives for SMART check（唤醒硬盘进行 SMART 检查）**，即可恢复每个周期都进行轮询（v0.9.4 的行为）。
- **历史迷你趋势图**：在仪表盘中直接展示 CPU、内存、I/O 等待以及各硬盘温度的变化趋势。
- **磁盘空间**：按挂载点显示空间使用情况，并根据预设阈值自动变色。
- **系统**：CPU、内存、平均负载、I/O 等待、运行时长、平台识别、**CPU 封装温度** 和 **主板温度**（通过 `/sys/class/hwmon` 自动检测；在仪表盘顶部与 CPU、内存、I/O 指标并列，以按数值区间显示不同颜色的仪表组件展示；若系统未提供相应传感器，例如群晖 DSM，则自动隐藏）。
- **Docker**：列出容器及其状态、运行时长。
- **ZFS 存储池健康状态**：存储池状态、vdev 树、数据巡检（scrub）和数据重建（resilver）的状态、ARC 命中率、碎片率，以及包含压缩比的数据集列表。
- **UPS／电源**：通过 NUT 或 apcupsd（本地或远程）获取电池电量、负载、预计续航时间和功率；当 UPS 处于电池供电状态或电池电量不足时，发出严重级别的告警。
- **网络**：网卡协商速率、状态和 MTU。
- **日志**：筛选 dmesg 和 syslog 中的错误（ATA 错误、I/O 错误、介质错误）。
- **奇偶校验**（Unraid）：分析历史奇偶校验检查速度的变化趋势，并跟踪错误。
- **隧道**：Cloudflared 隧道状态（连接、路由）和 Tailscale 对等节点拓扑（IP、在线／离线、中继、出口节点）。支持检测宿主机上运行的 Tailscale 服务（检测所需的 Tailscale CLI 已内置于镜像中），也支持检测 Docker 容器中的 Tailscale；Cloudflared 支持检测 Docker 容器，检测宿主机程序则需要使用内置 `cloudflared` CLI 的自定义镜像。
- **Proxmox VE**：通过 PVE REST API 获取集群状态、节点（CPU、内存、运行时长）、VM 和 LXC（状态、资源）、存储池、HA 服务及近期任务／备份，并支持连接测试。
- **Kubernetes**：支持监控 k8s、k3s、EKS、GKE 和 AKS 集群，展示节点（状态、磁盘使用情况、Pod 容量）、按节点分组并细分命名空间的 Pod、Deployment、Service、PVC 和警告事件。支持集群内自动检测和外部令牌认证。*在 Kubernetes 中检测 Tailscale，需要通过 emptyDir 共享 `/var/run/tailscale` 的 sidecar pod；详见 [docs/tailscale-install-methods.md](docs/tailscale-install-methods.md)。*
- **操作系统更新检查**：将已安装的 Unraid 和 TrueNAS 版本与 GitHub 上的最新发行版进行比较。

<a id="analysis-engine"></a>

### 分析引擎

提供 20+ 条诊断规则，并自动进行交叉关联分析：

- UDMA CRC 错误 + 奇偶校验缓慢 → **根本原因：SATA 线缆故障**
- 高温 + 奇偶校验缓慢 → **温度过高导致降频**
- 无 SSD 缓存 + I/O 等待偏高 + Docker 容器 → **I/O 资源不足**
- 待重映射扇区 + 已重映射扇区 → **硬盘介质正在失效**
- 已重映射扇区对应 Backblaze 数据中 12.0 倍的故障率 → **立即更换**
- ZFS 存储池处于 DEGRADED 状态，且有 REMOVED 的 vdev → **已无冗余保护，需要更换硬盘**
- UPS 使用电池供电且剩余续航时间较短 → **启动安全关机流程（graceful shutdown）**
- 操作系统版本严重过时 → **存在安全漏洞风险**
- 以及更多规则……

<a id="alerts--incident-management"></a>

### 告警与事件管理

独立的 `/alerts` 页面提供：

- **当前告警**：确认、暂缓和取消暂缓告警，并查看每条告警的完整生命周期时间线。
- **事件时间线与关联分析**：在可选时间范围（24 小时／7 天／30 天）内，将告警与 CPU、内存、I/O 等待和硬盘温度关联分析。
- **趋势预测**：检测 SMART 计数器的恶化模式，提供紧迫程度评分、置信水平和奇偶校验风险标记。
- **通知历史**：查看 Webhook 投递日志，包括状态、错误详情，并支持自动刷新。
- **可拖动卡片**：调整顺序、折叠、切换卡片显示状态，并保存布局。

<a id="service-checks"></a>

### 服务检查

独立的 `/service-checks` 页面提供可用性监控：

- 支持 **HTTP/HTTPS**、**TCP**、**DNS**、**Ping/ICMP**、**SMB**、**NFS**、**Traceroute（路由跟踪）** 和 **Speed Test（测速）** 检查类型。
- **路由跟踪检查**：通过镜像内置的 `mtr` 逐跳探测到目标的路径，显示每一跳的延迟和丢包率。
- **速度检查**：分别将下载和上传速率与运营商约定的对应速率进行比较，并支持设置允许的误差范围。结果分为三种状态：绿色（两项均达标）、橙色（性能下降）、红色（两项均未达标）。
- **保存前测试**：编辑器中的 **Test（测试）** 按钮可执行一次检查。路由跟踪会随探测进度显示各跳；速度检查会显示与仪表盘测速卡片相同的实时图表，随后展示判定结果及引擎测得的数据。
- **每项检查均可配置间隔**（30 秒至 1 小时），并由独立的调度循环执行。
- **心跳状态卡片**：用彩色圆点表示各服务近期的检查状态，HTTP 目标还会显示网站图标。
- **分页日志表**：支持按检查名称、状态和时间范围筛选。
- 跟踪历史响应时间和可用率。

<a id="drive-replacement-planner"></a>

### 硬盘更换规划

独立的 `/replacement-planner` 页面提供主动式硬盘生命周期管理：

- **逐盘健康评分**：综合硬盘使用年限、温度、SMART 属性和 Backblaze 年化故障率（2025 年第四季度数据，超过 33.7 万块硬盘）进行评分。
- **紧迫程度分类**：Replace Now（立即更换）、Replace Soon（尽快更换）、Monitor（持续观察）、Healthy（健康），并以不同颜色的卡片展示。
- **浴盆曲线老化模型**：在早期失效期（不足 6 个月）和磨损失效期（超过 4 年）提高故障倍率。
- **逐盘成本估算**：在 Settings 中配置每 TB 成本，显示单盘与总更换成本。
- **风险因素**：列出每块硬盘的具体风险（使用年限、温度、已重映射扇区、通电时长）。
- **剩余寿命估算**：根据当前使用年限和额定寿命，估算剩余使用年数。

<a id="backup-monitoring"></a>

### 备份监控

当 NAS Doctor 容器能够访问以下工具的 CLI 时，会自动检测并跟踪其备份：

- **Borg**、**Restic**、**Proxmox Backup Server（PBS）**、**Duplicati**：每次扫描通过 `exec.LookPath` 探测。
- **Duplicacy**：直接读取磁盘文件，无需 `duplicacy` 可执行文件（自 v0.10.0 起支持）。在 **Settings → Advanced → Backup Monitors → Duplicacy（设置 → 高级 → 备份监控 → Duplicacy）** 中配置 CLI 仓库或 saspus/duplicacy-web 缓存布局。详见下方的 [Duplicacy 监控](#duplicacy-monitoring-disk-read-no-binary-required)。
- 跟踪最近备份时间、大小、快照数量、耗时和加密状态。
- **备份过期告警**：超过 24 小时发出警告，超过 48 小时发出严重告警，也会对备份失败发出告警。

> **注意**：NAS Doctor Docker 镜像未内置 Restic、PBS 和 Duplicati 的可执行文件。Borg **已内置**（自 v0.9.10 起；参见下方的外部 Borg 监控），通过只读绑定挂载即可访问宿主机管理的仓库，无需自定义镜像。**Duplicacy** 完全不需要可执行文件（直接读取磁盘文件；参见下方的 Duplicacy 监控）。对于 Restic／PBS／Duplicati，除非你在容器内安装相应工具的 CLI（使用自定义 Dockerfile），或在与 NAS Doctor 共享卷／网络的其他容器中运行该工具，否则仪表盘的备份区域会保持为空。

<a id="external-borg-monitoring-host-managed-repos"></a>

#### 外部 Borg 监控（宿主机管理的仓库）

如果 Borg 运行在**宿主机**上（例如 Unraid User Scripts 或群晖任务计划），而非 NAS Doctor 容器内部，你仍然可以监控它。镜像已内置 Borg，因此**无需从宿主机挂载可执行文件**，只需以**只读**方式绑定挂载仓库路径。

> NAS Doctor 使用 `borg --bypass-lock`，避免向仓库写入，因此可以安全使用只读挂载。理论上唯一可能出现的竞态情况是：宿主机正在执行 `borg create` 时，NAS Doctor 同时读取该仓库。这只会导致显示的归档数量暂时滞后，下一次扫描即可更新，不会造成数据损坏。

在 **Settings → Advanced → Backup Monitors → Borg（设置 → 高级 → 备份监控 → Borg）** 中配置：

1. **Repo Path（仓库路径）**：容器内可见的 Borg 仓库路径。请先将宿主机上的仓库位置以**只读**方式（`:ro` 或 `Mode="ro"`）绑定挂载到容器。例如：宿主机 `/mnt/user/appdata/borg/repo` → 容器 `/mnt/user/appdata/borg/repo`（RO）。
2. **Label（标签）**：可选，仪表盘中的显示名称（例如 `Offsite`）。
3. **Passphrase Env Var（口令环境变量）**：可选，默认为 `BORG_PASSPHRASE`。填写保存仓库加密口令的 Docker 环境变量名称。NAS Doctor **从不保存口令本身**，只读取你在容器上设置的环境变量。
4. **SSH Key Path（SSH 密钥路径）**：可选，适用于 `ssh://` 远程仓库。填写容器内的绝对路径，并将密钥文件以只读方式绑定挂载。
5. **Binary Path（可执行文件路径）**：可选，用于指定其他可执行文件。留空则使用内置程序。替代程序**必须兼容 musl**，因为 Alpine 基础镜像无法运行链接到 glibc 的可执行文件。

每个条目都有 **Test** 按钮，可按需探测仓库。访问失败的仓库会在仪表盘中显示为红色错误卡片，并注明具体原因（`binary_not_found`、`repo_inaccessible`、`passphrase_rejected`、`ssh_timeout`、`corrupt_repo`、`unknown`），方便你快速识别需要处理的仓库。

**Unraid 配置示例**：宿主机通过 User Scripts 运行 Borg，加密仓库位于 `/mnt/user/appdata/borg/main`：

```
# 在 Unraid 的 nas-doctor Docker 配置中：
Path:  /mnt/user/appdata/borg/main  →  /mnt/user/appdata/borg/main (RO)
Env:   BORG_PASSPHRASE              =  <your-passphrase>
```

随后进入 Settings → Advanced → Backup Monitors → Borg → Add Borg repo（添加 Borg 仓库）：

```
Enabled:           on
Label:             Main
Repo Path:         /mnt/user/appdata/borg/main
Binary Path:       （留空，使用内置 borg）
Passphrase Env:    BORG_PASSPHRASE
SSH Key Path:      （留空，本地仓库）
```

点击 **Test** 进行验证；成功时返回归档数量，失败时返回具体错误原因。无需重启容器，仓库会在下一个扫描周期出现在仪表盘中。

<a id="duplicacy-monitoring-disk-read-no-binary-required"></a>

#### Duplicacy 监控（读取磁盘文件，无需可执行文件）

无论是标准 Duplicacy CLI 安装，还是常用的 [saspus/duplicacy-web](https://hub.docker.com/r/saspus/duplicacy-web) 容器，都通过**读取 Duplicacy 在磁盘上与仓库缓存一同保存的 JSON 快照文件**进行监控。**不会调用 `duplicacy` 可执行文件，不会启动子进程，也不会发起网络请求**。Duplicacy CLI 采用源码可查看（source-available）许可证，因此这里不随镜像分发其可执行文件；上述两种部署方式均可使用这种监控方式。

在 **Settings → Advanced → Backup Monitors → Duplicacy** 中配置：

1. **Kind（类型）**：`cli-repo`（标准 CLI 安装）或 `web-cache`（saspus/duplicacy-web 容器布局）。
2. **Path（路径）**：NAS Doctor 容器内可见的仓库根目录（cli-repo）或缓存根目录（web-cache）。请以**只读**方式绑定挂载 Duplicacy 路径；直接读取磁盘文件的方式可以安全使用只读挂载。
3. **Storage ID（存储 ID）**：仅用于 `kind=web-cache`，表示 saspus 容器在 Path 下为相应仓库写入的子目录名称。
4. **Stale After (days)（过期天数）**：如果仓库中最新快照的时间距今超过此阈值，则报告 `stale` 原因。默认 30 天，可逐项配置，以支持每日、每周和每月备份计划混用。
5. **Label（标签）**：可选，仪表盘中的显示名称。

每个条目都有 **Test** 按钮，会依据尚未保存的配置直接读取磁盘文件并立即显示结果，无需经历“保存 → 扫描 → 等待”的循环。原因代码限定为以下几种：

`ok` · `path_not_found` · `path_unreadable` · `not_a_duplicacy_repo` ·
`storage_id_not_found` · `no_snapshots_yet` · `stale` · `corrupt_snapshot`

仪表盘组件为每个配置条目显示一行，包含类型标签（`DUPLICACY:CLI-REPO`／`DUPLICACY:WEB-CACHE`）、按原因代码着色的状态标记（ok=成功、no_snapshots_yet=信息、stale=警告，其余=错误）、快照数量和距上次备份的时长。如果在磁盘上检测到锁或 `incomplete` 标记，还会额外显示独立的 `RUNNING` 标记。访问失败的条目会显示为红色错误卡片，注明具体原因，方便区分路径配置错误、缺少 Storage ID，还是新仓库尚未运行过备份。

每个条目的 Prometheus Gauge 指标：
`nasdoctor_backup_duplicacy_snapshots_total{label="…"}`、
`_last_backup_age_seconds{label="…"}`、
`_last_backup_size_bytes{label="…"}`、
`_status{label="…",reason="…"}`（条目当前原因代码对应的值为 1，其他原因代码为 0；与 `nasdoctor_speedtest_engine` 的约定相同）。

<a id="network-speed-test"></a>

### 网络测速

<p><img src="screenshots/speedtest-live.jpg" alt="测试中的测速卡片：延迟、下载和上传步骤，实时读数，下载和上传速度（Mbps）图表，以及下方的历史图表" width="380"></p>

- **测速过程实时展示**：仪表盘测速卡片会显示 `LATENCY → DOWNLOAD → UPLOAD`（延迟 → 下载 → 上传）步骤、实时下载／上传速度和延迟读数，以及随测试更新的下载／上传速度图表（Mbps）。测试结束后，本次结果和图表会保留在卡片中，直到你将其关闭；卡片底层的数值和历史记录仍会继续更新。Settings 中速度检查的 **Test** 按钮使用相同面板。进度通过服务器发送事件（Server-Sent Events，SSE）传输，因此多个标签页可以观看同一测试，测试中途打开的页面也会回放已有采样。**Cancel（取消）** 会及时停止引擎、关闭事件流，并重置表示“测试进行中”的 Prometheus Gauge 指标。**经反向代理访问时的实时更新**：采用尽力而为的方式，不保证一定能够实时显示。部分配置（尤其是 Cloudflare Access／Tunnel）会缓冲事件流，直到测试结束才释放，因此图表只会在最后一次性显示。直接通过局域网访问时，事件流可顺畅传输；两种访问方式的最终结果和逐次采样历史相同。
- 测速卡片上的 **Run now（立即运行）** 按钮具有幂等性：启动一次测试，或连接到正在进行的测试。手动运行不受测速计划中的 Disabled（禁用）设置限制，因为该设置只控制*计划测速*。
- **引擎**：主要使用内置的 `showwin/speedtest-go`（纯 Go 实现）；如果主引擎出错，则回退到内置的 Ookla CLI。每条历史记录都会注明所用引擎，仪表盘在最新结果旁显示 `via {engine}`。各记录的引擎信息还会通过 Prometheus 和快照 API 导出，便于你自行比较不同引擎的测量结果。
- **逐次采样历史**：每次测速的吞吐量采样均保存到 `speedtest_samples` 表中。在 `/service-checks` 页面展开任意历史 `type=speed` 条目，即可查看该测试期间吞吐量的变化。
- **从历史记录填充初始状态**：全新安装或冷启动时，会从历史记录显示最近一次成功测速，并附上“Last test: X ago”（距上次测速已过 X）的相对时间说明，无需等待下一次定时任务。
- **历史图表**：展示下载和上传速度，支持 1H／1D／1W（1 小时／1 天／1 周）时间范围。如果所选时间范围内的测速记录少于两次，图表会自动将显示的时间范围扩展至 24 小时、7 天或 30 天，并在图表下方注明。
- **测速计划**：默认每天 03:00 执行一次。Settings → Speed Test（设置 → 测速）提供从每 30 分钟到每月一次的选项；每日、每周和每月执行时可指定时间。对于按流量计费的连接，可选择 **Disabled**。全新安装还会创建 **Internet Speed（互联网速度）** 服务检查，用于报告最新结果，并可分别与运营商约定的下载和上传速率比较。
- 显示下载速度、上传速度、延迟、抖动，以及服务器名称、ISP 和外部 IP。

<a id="tunnel-monitoring"></a>

### 隧道监控

自动检测并监控远程访问隧道：

- **Cloudflared**：隧道状态、连接数和入口路由；开箱即支持检测 Docker 容器。检测宿主机程序需要使用内置 `cloudflared` CLI 的自定义镜像，默认镜像只内置 `tailscale`，未内置 `cloudflared`。
- **Tailscale**：**通过绑定挂载能够访问宿主机守护进程套接字目录 `/var/run/tailscale` 时**，展示完整的对等节点拓扑，包括在线状态、IP、操作系统、中继区域、发送／接收字节数和出口节点状态。如果 CLI 与守护进程版本不匹配，无法获取 JSON 输出，则回退到解析纯文本 `tailscale status` 输出，此时只能获取部分信息（IP、主机名、操作系统、在线状态）。守护进程不可达时，仪表盘会提示需要挂载的内容。
- Docker 容器检测默认匹配 `tailscale`。可通过环境变量 `NAS_DOCTOR_TAILSCALE_CONTAINER_NAMES=ts-sidecar,mullvad-ts,vpn` 扩展到自定义名称的边车容器；名称以逗号分隔，按不区分大小写的子字符串匹配。
- 所有主题的仪表盘均包含此区域，为每条隧道／每个对等节点显示状态圆点。
- 各安装方式（宿主机程序、Docker、Kubernetes 边车）的完整支持情况表见 [docs/tailscale-install-methods.md](docs/tailscale-install-methods.md)。

<a id="top-processes"></a>

### 高占用进程

实时监控进程，并识别其所属 Docker 容器：

- **仪表盘区域**：按 CPU 使用率排列前 10 个进程，通过 Linux cgroup 匹配，为每个进程标注所属 Docker 容器名称。
- **点击跳转**：点击任意进程，跳转至 `/stats` 上对应的 CPU 历史图表。
- **历史图表**：在 `/stats` 上查看各进程的 CPU 使用率时间序列，支持 **1H／1D／1W／1M**（1 小时／1 天／1 周／1 个月）范围选择。
- **容器归属**：读取 `/proc/PID/cgroup`，将进程匹配到 Docker 容器。支持 cgroup v1（Unraid）和 cgroup v2（TrueNAS SCALE）。
- **每 5 分钟采集**：进程统计数据与容器统计数据一起每 5 分钟采集一次。
- **告警规则**：可为各进程配置 `cpu_above` 和 `mem_above` 阈值。

> **需要 `--pid=host`**（或 Compose 中的 `pid: host`）；否则容器只能看到自身的进程。

<a id="parity-detail"></a>

### 奇偶校验详情

独立的 `/parity` 页面展示完整的奇偶校验检查历史：

- 覆盖全部历史检查的**速度趋势图**。
- 每次检查均有**可展开的详情卡片**（耗时、速度、错误、操作、阵列大小、退出码）。
- 仪表盘以**可滚动浏览的胶囊状状态标签**展示，按最新记录优先排列，替代旧表格。

<a id="notification-rules"></a>

### 通知规则

通过下拉菜单构建精细的通知规则，无需 YAML 或复杂的策略语法：

- **13 个类别**：诊断结果、磁盘空间、硬盘温度、SMART 健康状态、服务检查、进程、奇偶校验、UPS／电源、Docker、系统、ZFS、隧道、平台更新。
- **条件下拉菜单**随类别变化；例如 SMART 提供“SMART 健康状态检查未通过”“已重映射扇区超过阈值”“待重映射扇区超过阈值”“CRC 错误超过阈值”“通电时长超过阈值”。
- **基于实时数据选择目标**：从最新扫描结果填充的下拉菜单中，选择特定硬盘、服务、容器、ZFS 存储池或隧道。
- **阈值设置**：设置具体数值，例如磁盘剩余空间低于 10%、温度高于 55°C。
- **5 个一键预设**：严重告警、硬盘健康监控、服务可用性、电源保护、存储警告。
- **Quiet Hours（免打扰时段）**：在每日指定时间段内停止发送通知，告警仍会记录。
- **Maintenance Windows（维护窗口）**：按主机名安排停止通知的时段。
- **Default Cooldown（默认冷却时间）**：统一设置每条规则的去重时间窗口。

<a id="api-key-authentication"></a>

### API 密钥认证

每个实例都有独立的 API 密钥系统，用于保护多服务器通信：

- 在 Settings 中**生成／复制／撤销**密钥，密钥格式为 `nd-{uuid}`。
- 设置密钥后，所有 `/api/v1/*` 端点均受保护，包括 `/health`。
- 仪表盘界面不受此限制，同源请求可直接通过。
- 保存前，多服务器连接测试会使用 API 密钥进行端到端验证。
- Docker HEALTHCHECK 和 K8s 探针通过 TCP 端口检查，无需认证。

<a id="multi-server-fleet-monitoring"></a>

### 多服务器监控（Fleet）

在 `/fleet` 的可视化拓扑中监控所有 NAS Doctor 实例：

- **可视化拓扑**：中央主节点与连接的远程服务器。
- 每台服务器显示：平台图标、主机名、IP、NAS Doctor 版本、运行时长、健康状态和诊断结果数量。
- **自动检测连接类型**：区分局域网（私有 IP）和公网主机名，并检测隧道类型（Cloudflare、Tailscale）。
- 为每台服务器设置**自定义认证请求头**，用于 Cloudflare Access、Authelia 等。
- **Test Connection（测试连接）**：端到端验证 NAS Doctor 特征标识和 API 密钥。
- 添加服务器时**自动创建服务检查**。
- 通过可折叠表单**编辑／移除**各服务器。
- **Open Dashboard（打开仪表盘）** 链接可直接访问远程实例。
- 多服务器轮询需要 API 密钥。

<a id="integrations"></a>

### 集成

| 集成 | 方式 |
|---|---|
| **Prometheus** | 抓取 `/metrics`，提供超过 120 个 Gauge 指标，涵盖系统（含 CPU／主板温度）、磁盘、SMART、Docker、网络、UPS、ZFS、GPU、服务、奇偶校验、隧道、Proxmox、Kubernetes、备份、测速（含实时测试进行状态、所用引擎和采样数量指标）及诊断结果。 |
| **Grafana** | 通过 Prometheus 数据源连接。 |
| **Discord** | Webhook 通知包含丰富的嵌入内容、严重程度颜色和诊断结果详情。 |
| **Slack** | Webhook 通知包含消息区块、各严重程度计数和主要诊断结果。 |
| **Gotify** | 原生推送通知，支持优先级映射。 |
| **Ntfy** | 带优先级和标签的推送通知。 |
| **通用 HTTP** | 使用带 HMAC-SHA256 签名的 JSON 载荷实现自定义集成。 |

---

<a id="quick-start"></a>

## 快速开始

<a id="docker-compose-recommended"></a>

### Docker Compose（推荐）

```yaml
services:
  nas-doctor:
    image: ghcr.io/mcdays94/nas-doctor:latest
    container_name: nas-doctor
    privileged: true          # 访问 SMART 时必需
    pid: host                 # 高占用进程监控必需，用于查看宿主机进程
    network_mode: host
    volumes:
      - nas-doctor-data:/data
      - /var/run/docker.sock:/var/run/docker.sock:ro
      - /var/log:/host/log:ro
      - /dev:/dev:ro                       # SMART 设备访问
      - /sys:/sys:ro                       # GPU 遥测
      # 挂载存储卷，请按所用平台配置：
      - /mnt:/host/mnt:ro                  # Unraid、TrueNAS
      # - /volume1:/host/volume1:ro        # 群晖，逐个添加存储卷
      # - /volume2:/host/volume2:ro        # 群晖
      # Unraid 专用，其他平台请省略：
      - /boot:/host/boot:ro
      - /etc/unraid-version:/etc/unraid-version:ro
      - /var/local/emhttp:/var/local/emhttp:ro  # 硬盘槽位映射，用于合并硬盘视图
      # 如果使用 Tailscale 并需要对等节点拓扑，则必须挂载，适用于任何平台：
      - /var/run/tailscale:/var/run/tailscale:ro  # 通过宿主机守护进程套接字检测 Tailscale 对等节点
    devices:
      - /dev/dri:/dev/dri                  # GPU 监控（Intel/AMD）
    environment:
      - TZ=Europe/Lisbon
      - NAS_DOCTOR_INTERVAL=30m
    restart: unless-stopped

volumes:
  nas-doctor-data:
```

```bash
docker compose up -d
```

随后打开 `http://your-nas:8060`。Unraid、群晖和 TrueNAS 的具体配置见下方各平台章节。

<a id="unraid--docker-ui-setup"></a>

### Unraid — 通过 Docker 界面配置

1. 进入 **Docker** 标签页，向下滚动，点击 **Add Container（添加容器）**。
2. 填写以下字段：

| 字段 | 值 |
|---|---|
| **Name（名称）** | `nas-doctor` |
| **Repository（镜像）** | `ghcr.io/mcdays94/nas-doctor:latest` |
| **Icon URL（图标 URL）** | `https://raw.githubusercontent.com/mcdays94/nas-doctor/main/icons/icon3.png` |
| **WebUI** | `http://[IP]:8060/`（如果修改下方的监听端口，也要相应更新此地址） |
| **Network Type（网络类型）** | `Host` |
| **Privileged（特权模式）** | `On`（**必需**，SMART 需要访问原始设备） |
| **Extra Parameters（额外参数）** | `--pid=host`（高占用进程监控查看宿主机进程时**必需**） |

3. 添加以下**路径映射**，每添加一项都点击“Add another Path, Port, Variable...（添加其他路径、端口、变量……）”：

| 名称 | 容器路径 | 宿主机路径 | 模式 | 用途 |
|---|---|---|---|---|
| Data（数据） | `/data` | `/mnt/user/appdata/nas-doctor` | RW | 数据库、配置和备份 |
| Docker Socket（Docker 套接字） | `/var/run/docker.sock` | `/var/run/docker.sock` | RO | 容器监控 |
| Boot Config（启动配置） | `/host/boot` | `/boot` | RO | 奇偶校验日志和 Unraid 识别 |
| System Logs（系统日志） | `/host/log` | `/var/log` | RO | dmesg、syslog 分析 |
| Host Mounts（宿主机挂载） | `/host/mnt` | `/mnt` | RO | 逐盘空间监控 |
| Unraid Version（Unraid 版本） | `/etc/unraid-version` | `/etc/unraid-version` | RO | 操作系统更新检测 |
| Disk Slots（硬盘槽位） | `/var/local/emhttp` | `/var/local/emhttp` | RO | 合并硬盘视图中的槽位映射 |
| Device Nodes（设备节点） | `/dev` | `/dev` | RO | SMART 和 GPU 设备访问 |
| Sysfs | `/sys` | `/sys` | RO | GPU 遥测和硬盘映射 |
| Tailscale Socket（Tailscale 套接字） | `/var/run/tailscale` | `/var/run/tailscale` | RO | **使用 Tailscale 时必需**，用于检测对等节点拓扑，适用于 `tailscale-nas-util` 插件或采用 `network_mode: host` 的 Tailscale 容器。不使用 Tailscale 时可留空。缺少此挂载时，仪表盘会显示“Unreachable（不可达）”提示，而非对等节点数据。 |

4. 添加以下**变量**：

| 键 | 值 |
|---|---|
| `TZ` | 你的时区，例如 `Europe/Lisbon`、`America/New_York`。 |
| `NAS_DOCTOR_LISTEN` | HTTP 监听地址，默认为 `:8060`。如果端口 8060 已被占用，可改为 `:8067` 等。也可以填写纯端口号，`8067` 会自动规范为 `:8067`。由于容器使用宿主机网络，监听端口通过此变量设置，而非 Docker 端口映射。 |

5. 点击 **Apply（应用）**。

随后打开 `http://your-unraid-ip:8060`，或访问你设置的其他端口。

> **重要**：必须启用特权模式，并配置 Host Mounts 卷（`/mnt:/host/mnt:ro`）。未启用特权模式时无法读取 SMART 数据；未挂载 `/mnt` 时无法显示逐盘空间。
>
> **修改端口**：容器使用宿主机网络，因此模板中的“Web UI Port（网页界面端口）”字段设置的是 `NAS_DOCTOR_LISTEN`，而非 Docker 端口映射。修改后还需在 Unraid 中更新 WebUI URL（容器设置 → Advanced View → WebUI），确保点击图标时打开正确的端口。

<a id="synology-dsm--container-manager"></a>

### 群晖 DSM — Container Manager

通过 **Container Manager（容器管理器）** 部署，或通过 SSH 使用 Docker 部署。

```yaml
services:
  nas-doctor:
    image: ghcr.io/mcdays94/nas-doctor:latest
    container_name: nas-doctor
    privileged: true
    pid: host
    network_mode: host
    volumes:
      - /volume1/docker/nas-doctor:/data
      - /var/run/docker.sock:/var/run/docker.sock:ro
      - /var/log:/host/log:ro
      - /dev:/dev:ro                       # 访问 SMART 设备时必需
      - /volume1:/host/volume1:ro
      - /volume2:/host/volume2:ro          # 按需添加更多存储卷
    environment:
      - TZ=Europe/Lisbon
      - NAS_DOCTOR_INTERVAL=30m
    restart: unless-stopped
```

随后打开 `http://your-synology-ip:8060`。

> **群晖注意事项**：
> - 访问 SMART **需要启用特权模式**，因为 `smartctl` 需要通过 `SYS_RAWIO` 能力访问原始设备。
> - **挂载 `/dev:/dev:ro`**：群晖硬盘槽位使用 `/dev/sata*` 设备节点，容器必须能够访问这些节点才能查询 SMART。NAS Doctor 会自动尝试 SCSI 到 ATA 转换（`--device=sat`）作为回退方式。
> - 挂载每个需要监控的 `/volume<#>`；群晖使用 `/volume1`、`/volume2` 等路径，而非 `/mnt`。
> - 群晖没有 `/boot` 或 `/etc/unraid-version`，请省略这些挂载。
> - 奇偶校验分析仅适用于 Unraid，在群晖上会自动跳过。
> - 如果 SMART 仍显示警告，可尝试显式添加 `cap_add: [SYS_RAWIO]`。

<a id="truenas-scale"></a>

### TrueNAS SCALE

通过 **Apps（应用）** 部署，或通过 SSH 使用 Docker Compose 部署。

```yaml
services:
  nas-doctor:
    image: ghcr.io/mcdays94/nas-doctor:latest
    container_name: nas-doctor
    privileged: true
    pid: host
    network_mode: host
    volumes:
      - /mnt/pool/appdata/nas-doctor:/data
      - /var/run/docker.sock:/var/run/docker.sock:ro
      - /var/log:/host/log:ro
      - /mnt:/host/mnt:ro
      - /dev:/dev:ro                       # 访问 SMART 设备时必需
      - /sys:/sys:ro                       # GPU 监控必需
    devices:
      - /dev/dri:/dev/dri                  # Intel/AMD GPU 访问，如适用
    environment:
      - TZ=America/New_York
      - NAS_DOCTOR_INTERVAL=30m
    restart: unless-stopped
```

随后打开 `http://your-truenas-ip:8060`。

> **TrueNAS 注意事项**：
> - 访问 SMART **需要启用特权模式**。
> - **挂载 `/dev:/dev:ro`** 以访问 SMART 设备，挂载 **`/sys:/sys:ro`** 以获取 GPU 遥测数据。
> - **`/dev/dri`** 设备直通可启用 Intel 核显监控（使用率、温度、功率）。
> - ZFS 存储池健康状态、数据巡检状态、ARC 命中率和数据集列表会自动生效。
> - 挂载 `/mnt`，即可查看所有存储池／数据集的空间使用情况。
> - 从 `/etc/version` 或 `/etc/os-release` 检测 TrueNAS 版本，无需 API 认证。
> - 奇偶校验分析仅适用于 Unraid，在 TrueNAS 上会自动跳过。
> - 配置 NUT 后即可使用 UPS 监控；TrueNAS 内置 NUT 支持。

<a id="kubernetes-k3s--k8s"></a>

### Kubernetes（k3s / k8s）

通过 kubectl 或 GitOps（ArgoCD／Flux）部署：

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: nas-doctor
  namespace: nas-doctor
spec:
  replicas: 1
  selector:
    matchLabels:
      app: nas-doctor
  template:
    metadata:
      labels:
        app: nas-doctor
    spec:
      serviceAccountName: nas-doctor
      containers:
        - name: nas-doctor
          image: ghcr.io/mcdays94/nas-doctor:latest
          ports:
            - containerPort: 8060
          env:
            - name: TZ
              value: Europe/Lisbon
          volumeMounts:
            - name: data
              mountPath: /data
          livenessProbe:
            tcpSocket:
              port: 8060
      volumes:
        - name: data
          persistentVolumeClaim:
            claimName: nas-doctor-data
```

还需要具有 nodes、pods、deployments、services、namespaces、PVC 和 events 读取权限的 ServiceAccount 和 ClusterRole。完整示例见 [K8s 完整部署清单](https://github.com/mcdays94/k3s-gitops/tree/main/apps/nas-doctor)。

> **K8s 注意事项**：
> - 在 Settings → Kubernetes（设置 → Kubernetes）中启用 **In-cluster auto-detect（集群内自动检测）**，使用挂载的服务账号令牌。
> - `view` ClusterRole **权限不足**，因为节点是集群级资源，请使用自定义 ClusterRole。
> - 多架构镜像支持 amd64 和 arm64（树莓派）节点。
> - 无需 Docker 套接字，K8s 集成直接使用 API。
> - 每个节点的磁盘使用信息来自 `ephemeral-storage` 容量。

<a id="proxmox-via-ubuntu-vm--lxc"></a>

### Proxmox（通过 Ubuntu VM / LXC）

在 Proxmox 虚拟机中，通过 Portainer 或 Docker Compose 部署：

```yaml
services:
  nas-doctor:
    image: ghcr.io/mcdays94/nas-doctor:latest
    container_name: nas-doctor
    privileged: true
    pid: host
    network_mode: host
    restart: unless-stopped
    environment:
      - TZ=Europe/Lisbon
    volumes:
      - nas-doctor-data:/data
      - /var/log:/host/log:ro
      - /var/run/docker.sock:/var/run/docker.sock:ro

volumes:
  nas-doctor-data:
```

随后进入 Settings → Proxmox VE（设置 → Proxmox VE），填写 PVE API URL（`https://proxmox:8006`），创建 API 令牌（Datacenter → Permissions → API Tokens，即数据中心 → 权限 → API 令牌，取消勾选 Privilege Separation／权限分离），然后点击 Test Connection。

> **Proxmox 注意事项**：
> - 自动接受 PVE 自签名证书。
> - 节点筛选下拉菜单由连接测试结果自动填充。
> - 支持显示别名，方便识别，例如“Proxmox LDN”。
> - 分析器可检测：节点离线、内存占用达到严重阈值、存储已满、备份过期、HA 错误和任务失败。
> - SMART 监控需要将物理硬盘直通给 VM／LXC。

<a id="build-from-source"></a>

### 从源码构建

```bash
git clone https://github.com/mcdays94/nas-doctor.git
cd nas-doctor
go build -o nas-doctor ./cmd/nas-doctor
./nas-doctor -listen :8060 -data ./data -interval 30m
```

如果没有 NAS，也可以通过 `./nas-doctor -demo -listen :8060` 使用生成的数据运行应用，进行界面开发。下方截图均来自该模式。

---

<a id="screenshots"></a>

## 截图

以下截图来自演示模式，该模式使用生成的数据展示界面。

<p>
  <img src="screenshots/service-checks-page.jpg" alt="服务检查：各项检查（ping、HTTP、DNS、NFS、SMB、速度）的可用性卡片和检查日志" width="380">
  <img src="screenshots/alerts-page.jpg" alt="告警：各严重程度的数量和当前告警，支持确认、暂缓、时间线及关联分析操作" width="380">
</p>
<p>
  <img src="screenshots/fleet-page.jpg" alt="多服务器监控：NAS Doctor 实例的网络拓扑和健康状态矩阵" width="380">
  <img src="screenshots/stats-page.jpg" alt="统计：硬盘概况，以及 CPU、内存、I/O 等待和负载图表" width="380">
</p>
<p>
  <img src="screenshots/settings-page.jpg" alt="设置：分区标签页、扫描间隔、主题和应用图标" width="380">
  <img src="screenshots/settings-advanced-scans.jpg" alt="设置 → 高级：SMART 以及 Docker、Proxmox、Kubernetes、ZFS、GPU 各子系统的扫描间隔，默认均使用全局间隔" width="380">
</p>
<p>
  <img src="screenshots/parity-page.jpg" alt="奇偶校验：各次检查的速度趋势和历史记录" width="380">
  <img src="screenshots/disk-detail.jpg" alt="硬盘详情：健康评分、身份标记、SMART 属性及其趋势" width="380">
</p>
<p>
  <img src="screenshots/dashboard-processes.jpg" alt="仪表盘：标明容器归属的高占用进程，旁边是服务检查、隧道和奇偶校验历史" width="380">
  <img src="screenshots/stats-process-history.jpg" alt="统计：各进程 CPU 使用率历史和容量预测" width="380">
</p>
<p>
  <img src="screenshots/planner-page.jpg" alt="硬盘更换规划：各硬盘的紧迫程度、风险因素、已用寿命和预计剩余时间" width="380">
</p>

---

<a id="settings"></a>

## 设置

所有选项均可在 `/settings` 网页界面中配置，页面采用滚动时保持可见的分区导航组织内容：

- **General（常规）**：扫描间隔（预设或自定义，附 cron 预览）、主题选择、应用图标。
- **API Key（API 密钥）**：生成、复制或撤销用于保护 `/api/v1/*` 的密钥。
- **Webhooks**：添加／移除／测试 Discord、Slack、Gotify、Ntfy 或通用 HTTP Webhook，可选自定义请求头和 HMAC 签名。
- **Notification Rules（通知规则）**：通过下拉菜单构建规则，支持 13 个类别、实时目标选择、阈值输入、一键预设、免打扰时段和维护窗口。
- **Service Checks（服务检查）**：HTTP、TCP、DNS、Ping/ICMP、SMB、NFS、路由跟踪和测速检查，每项均可配置间隔（30 秒至 1 小时），并提供 **Test** 按钮。
- **Proxmox VE** 和 **Kubernetes**：API 连接配置及连接测试。
- **Log Forwarding（日志转发）**：每次扫描后将结果转发到 **Loki**、**syslog**（UDP／TCP）或任意 **HTTP JSON** 端点，可配置自定义请求头、标签和载荷格式（完整、仅诊断结果、摘要）。
- **Data Lifecycle（数据生命周期）**：快照保留天数、数据库大小上限和通知日志保留设置。
- **Drive Replacement Cost（硬盘更换成本）**：更换规划中用于估算的每 TB 成本。
- **Fleet（多服务器）**：添加／移除远程 NAS Doctor 实例，可选 API 密钥认证。
- **Dashboard Sections（仪表盘区域）**：切换各区域的显示状态，包括 SMART、Docker、ZFS、UPS、奇偶校验、网络、隧道等。
- **Speed Test**：设置计划测速执行时间，默认每天 03:00 一次，也可禁用。
- **Automatic Backup（自动备份）**：计划数据库备份，可配置位置、间隔和保留数量。
- **Advanced（高级）**：SMART 扫描控制（唤醒硬盘、未读取 SMART 的最长天数）、各子系统扫描间隔、隐藏 Docker 容器，以及 Borg／Duplicacy 备份监控。

<a id="environment-variables"></a>

### 环境变量

| 变量 | 默认值 | 说明 |
|---|---|---|
| `NAS_DOCTOR_LISTEN` | `:8060` | HTTP 监听地址，支持 `:port`、`host:port` 或纯 `port`，纯端口号会自动规范化。 |
| `NAS_DOCTOR_DATA` | `/data` | SQLite 数据库目录。 |
| `NAS_DOCTOR_INTERVAL` | `30m` | 诊断扫描间隔。 |
| `NAS_DOCTOR_UPS_NAME` | （自动检测） | NUT UPS 名称，设置后跳过通过 `upsc -l` 自动检测。 |
| `NAS_DOCTOR_NUT_HOST` | （本地） | 远程 NUT 服务器主机，查询形式为 `upsname@host`。 |
| `NAS_DOCTOR_APCUPSD_HOST` | （本地） | 远程 apcupsd 守护进程的 `host:port`。 |
| `NAS_DOCTOR_TAILSCALE_SOCKET` | `/var/run/tailscale/tailscaled.sock` | 容器内 Tailscale 守护进程套接字路径。 |
| `NAS_DOCTOR_TAILSCALE_CONTAINER_NAMES` | （无） | 额外按 Tailscale 处理的容器名称，以逗号分隔，按不区分大小写的子字符串匹配，例如 `ts-sidecar,vpn`。 |
| `NAS_DOCTOR_HOST_LOG` | `/host/log` | 宿主机 `/var/log` 在容器内的挂载位置。 |
| `NAS_DOCTOR_HOST_BOOT` | `/host/boot` | 宿主机 `/boot` 在容器内的挂载位置（Unraid）。 |
| `NAS_DOCTOR_WEBHOOK_URL` | （无） | 启动时添加一个 Webhook，与 Settings 中已有的 Webhook 并存。 |
| `NAS_DOCTOR_WEBHOOK_TYPE` | `generic` | 上述 Webhook 的类型：`discord`、`slack`、`gotify`、`ntfy` 或 `generic`。 |
| `NAS_DOCTOR_CONFIG` | （无） | 启动时加载的可选 JSON 配置文件，与 `-config` 相同。 |
| `TZ` | `UTC` | 时区。 |

---

<a id="api-reference"></a>

## API 参考

| 端点 | 方法 | 说明 |
|---|---|---|
| `/api/v1/health` | GET | 健康检查：状态、版本、运行时长。 |
| `/api/v1/status` | GET | 服务器状态摘要，包含各区域的显示状态。 |
| `/api/v1/snapshot/latest` | GET | 最新诊断快照的完整内容。 |
| `/api/v1/snapshot/{id}` | GET | 按 ID 获取指定快照。 |
| `/api/v1/snapshots` | GET | 列出近期快照。 |
| `/api/v1/scan` | POST | 立即触发诊断扫描。 |
| `/api/v1/history/speedtest` | GET | 测速历史，查询参数：`?hours=N`。 |
| `/api/v1/history/processes` | GET | 进程 CPU／内存历史，查询参数：`?hours=N`。 |
| `/api/v1/history/containers` | GET | 容器统计历史，查询参数：`?hours=N`。 |
| `/api/v1/history/gpu` | GET | GPU 指标历史，查询参数：`?hours=N`。 |
| `/api/v1/settings` | GET/PUT | 读取／写入应用设置。 |
| `/api/v1/settings/test-webhook` | POST | 向 Webhook 发送测试通知。 |
| `/api/v1/settings/chart-range` | PUT | 保存仪表盘图表范围：1、24 或 168 小时。 |
| `/api/v1/settings/section-order` | PUT | 保存仪表盘区域布局。 |
| `/api/v1/settings/section-heights` | PUT | 保存仪表盘各区域高度。 |
| `/api/v1/proxmox/test` | POST | 测试 Proxmox VE API 连接。 |
| `/api/v1/kubernetes/test` | POST | 测试 Kubernetes API 连接。 |
| `/api/v1/backup-monitor/borg/test` | POST | 探测 Borg 仓库配置。 |
| `/api/v1/backup-monitor/duplicacy/test` | POST | 探测 Duplicacy 仓库或缓存配置。 |
| `/api/v1/icons` | GET | 获取可用应用图标及默认图标。 |
| `/api/v1/sparklines` | GET | 用于图表的系统与 SMART 历史精简数据。 |
| `/api/v1/history/system` | GET | 系统指标历史：CPU、内存、I/O。 |
| `/api/v1/disks` | GET | 列出所有硬盘及其 SMART 数据。 |
| `/api/v1/disks/{serial}` | GET | 单块硬盘详情及完整 SMART 历史。 |
| `/api/v1/disk-usage-history` | GET | 各挂载点的磁盘使用历史。 |
| `/api/v1/capacity-forecast` | GET | 各挂载点的空间增长速率预测。 |
| `/api/v1/replacement-plan` | GET | 硬盘更换规划：紧迫程度、风险因素、成本。 |
| `/api/v1/alerts` | GET | 列出告警，可按状态筛选。 |
| `/api/v1/alerts/{id}` | GET | 获取单条告警详情。 |
| `/api/v1/alerts/{id}/events` | GET | 告警生命周期的时间线事件。 |
| `/api/v1/alerts/{id}/ack` | POST | 确认告警。 |
| `/api/v1/alerts/{id}/unack` | POST | 取消确认告警。 |
| `/api/v1/alerts/{id}/snooze` | POST | 暂缓告警，使用 `until` 时间戳指定截止时间。 |
| `/api/v1/alerts/{id}/unsnooze` | POST | 取消暂缓告警。 |
| `/api/v1/incidents/timeline` | GET | 叠加系统指标的事件时间线。 |
| `/api/v1/incidents/correlation` | GET | 告警关联分析，包含发生前、发生期间和发生后的指标。 |
| `/api/v1/smart/trends` | GET | SMART 恶化趋势及风险评分。 |
| `/api/v1/notifications/log` | GET | Webhook 投递历史。 |
| `/api/v1/service-checks` | GET | 最新服务检查结果。 |
| `/api/v1/service-checks/history` | GET | 服务检查结果历史。 |
| `/api/v1/service-checks/run` | POST | 立即触发服务检查。 |
| `/api/v1/service-checks/test` | POST | 按指定配置执行一次检查，不保存配置。 |
| `/api/v1/service-checks/test-stream` | POST | 与 `/test` 相同，但为路由跟踪和速度检查提供 SSE 事件流，先实时发送各跳或采样数据，再发送结果。 |
| `/api/v1/service-checks/{key}` | DELETE | 移除某项检查已保存的历史。 |
| `/api/v1/speedtest/run` | POST | 启动测速，或连接到正在进行的测试。具有幂等性，返回 `{test_id, started_at, engine}`。 |
| `/api/v1/speedtest/stream/{test_id}` | GET | 通过 SSE 传输正在进行的测试进度。事件类型：`start`、`phase_change`、`sample`、`result`、`cancelled`、`error`、`end`。 |
| `/api/v1/speedtest/cancel/{test_id}` | POST | 取消正在进行的测速。 |
| `/api/v1/speedtest/samples/{test_id}` | GET | 已完成测试的逐次吞吐量采样值，以 JSON 数组返回，供 `/service-checks` 展开的日志迷你图表使用。 |
| `/api/v1/findings/dismiss` | POST | 从仪表盘忽略某条诊断结果。 |
| `/api/v1/findings/restore` | POST | 恢复被忽略的诊断结果。 |
| `/api/v1/db/stats` | GET | 数据库大小和行数。 |
| `/api/v1/backup` | GET/POST | 列出或触发数据库备份。 |
| `/api/v1/fleet` | GET | 所有远程服务器的汇总状态。 |
| `/service-checks` | GET | 服务检查仪表盘（HTML）。 |
| `/parity` | GET | 奇偶校验历史详情页（HTML）。 |
| `/api/v1/fleet/servers` | GET/PUT | 管理远程服务器列表。 |
| `/api/v1/fleet/test` | POST | 测试与远程服务器的连接。 |
| `/metrics` | GET | Prometheus 指标端点。 |

---

<a id="prometheus-metrics"></a>

## Prometheus 指标

所有指标均以 `nasdoctor_` 为前缀，完整列表如下：

<details>
<summary>展开指标列表（超过 120 个指标）</summary>

```
# 系统（14 个 Gauge 指标）
nasdoctor_system_cpu_usage_percent / _cpu_cores
nasdoctor_system_memory_used_bytes / _total_bytes / _used_percent
nasdoctor_system_swap_used_bytes / _total_bytes
nasdoctor_system_load_avg_1 / _5 / _15
nasdoctor_system_io_wait_percent / _uptime_seconds
nasdoctor_system_cpu_temp_celsius / _mobo_temp_celsius   # 无可用传感器时为 0

# 磁盘（标签：device、mountpoint、label）
nasdoctor_disk_used_bytes / _total_bytes / _used_percent

# SMART（标签：device、model、serial），每块硬盘 11 个 Gauge 指标
nasdoctor_smart_healthy / _temperature_celsius / _temperature_max_celsius
nasdoctor_smart_reallocated_sectors / _pending_sectors / _offline_uncorrectable
nasdoctor_smart_udma_crc_errors / _command_timeout / _spin_retry_count
nasdoctor_smart_power_on_hours / _size_bytes

# Docker（标签：name、image）
nasdoctor_docker_container_cpu_percent / _memory_bytes / _running
nasdoctor_docker_container_count

# 网络（标签：interface）
nasdoctor_network_interface_up / _mtu

# UPS（10 个 Gauge 指标）
nasdoctor_ups_battery_percent / _battery_voltage
nasdoctor_ups_input_voltage / _output_voltage / _load_percent
nasdoctor_ups_runtime_minutes / _wattage_watts / _temperature_celsius
nasdoctor_ups_on_battery / _low_battery

# ZFS（存储池标签：pool；数据集标签：dataset+pool）
nasdoctor_zfs_pool_healthy / _used_bytes / _total_bytes / _used_percent
nasdoctor_zfs_pool_fragmentation_percent / _scan_percent / _scan_errors
nasdoctor_zfs_pool_read_errors / _write_errors / _checksum_errors
nasdoctor_zfs_arc_size_bytes / _max_size_bytes / _hit_rate_percent
nasdoctor_zfs_arc_hits_total / _misses_total
nasdoctor_zfs_l2arc_size_bytes / _hit_rate_percent
nasdoctor_zfs_dataset_used_bytes / _avail_bytes / _compression_ratio

# 服务检查（标签：name、type、target）
nasdoctor_service_up / _response_ms / _consecutive_failures

# 奇偶校验（Unraid）
nasdoctor_parity_speed_mb_per_sec / _duration_seconds / _errors / _running

# 隧道
nasdoctor_tunnel_cloudflared_up / _connections （标签：name）
nasdoctor_tunnel_tailscale_node_online / _tx_bytes / _rx_bytes （标签：name、ip）

# Proxmox（标签：node / vmid+name+type+node / storage+node+type）
nasdoctor_proxmox_node_cpu_usage / _memory_used_bytes / _memory_total_bytes / _node_online
nasdoctor_proxmox_guest_cpu_usage / _memory_used_bytes / _memory_max_bytes / _guest_running
nasdoctor_proxmox_storage_used_bytes / _storage_total_bytes

# Kubernetes（标签：node / pod+namespace / deployment+namespace）
nasdoctor_k8s_node_ready / _node_pod_count
nasdoctor_k8s_pod_running / _pod_restarts
nasdoctor_k8s_deployment_ready_replicas / _deployment_desired_replicas

# GPU（标签：index、name、vendor），每块 GPU 10 个 Gauge 指标
nasdoctor_gpu_usage_percent / _mem_used_mb / _mem_total_mb / _mem_percent
nasdoctor_gpu_temperature_celsius / _power_watts / _power_max_watts / _fan_percent
nasdoctor_gpu_encoder_percent / _decoder_percent

# 备份（标签：provider、name）
nasdoctor_backup_last_success_timestamp / _size_bytes / _status

# 备份 — Duplicacy（标签：label；_status 另含 reason），每个条目 4 个 Gauge 指标
nasdoctor_backup_duplicacy_snapshots_total{label="…"}
nasdoctor_backup_duplicacy_last_backup_age_seconds{label="…"}     # 抓取时计算，在两次扫描之间单调递增
nasdoctor_backup_duplicacy_last_backup_size_bytes{label="…"}
nasdoctor_backup_duplicacy_status{label="…",reason="…"}           # 当前原因对应值为 1，其余为 0，原因限定为 8 种

# 测速
nasdoctor_speedtest_download_mbps / _upload_mbps / _latency_ms
nasdoctor_speedtest_in_progress                            # 测试进行中时为 1
nasdoctor_speedtest_engine{engine="speedtest_go|ookla_cli"}  # 最近一次成功测试所用引擎对应值为 1
nasdoctor_speedtest_samples_count{test_id="..."}           # 最近一次已完成测试的采样数量

# 诊断结果
nasdoctor_findings_critical_count / _warning_count
nasdoctor_findings_total{severity="critical|warning|info"}

# 其他
nasdoctor_update_available
nasdoctor_collection_duration_seconds / _last_collection_timestamp
```

</details>

---

<a id="supported-platforms"></a>

## 支持的平台

| 平台 | 状态 | 说明 |
|---|---|---|
| **Unraid** | ✅ 已测试 | 奇偶校验分析、阵列状态、硬盘标签、操作系统更新检查；维护者每日实际使用。 |
| **群晖 DSM** | ⚠️ 社区已测试 | `/volume<#>` 检测、`/dev/mapper/cachedev_*` 支持、SMART 健康状态解析。 |
| **TrueNAS SCALE** | ⚠️ 未测试 | 已内置 ZFS 存储池健康监控，但尚未在真实硬件上验证。 |
| **Proxmox VE** | ⚠️ 社区已测试 | PVE REST API 集成，包含集群、节点、VM／LXC 视图；作为多服务器监控中的远程实例实际使用。 |
| **Kubernetes**（k3s / k8s） | ⚠️ 社区已测试 | 集群内自动检测、ServiceAccount 和 ClusterRole 认证；已在 k3s 上测试。 |
| **QNAP QTS** | ⚠️ 未测试 | 预计可通过 Container Station 运行。 |
| **通用 Linux** | ⚠️ 未测试 | 任何安装了 Docker 的发行版。 |

> **Unraid** 每天都有实际使用测试。群晖、Proxmox 和 Kubernetes 已有社区反馈或作为远程实例实际使用的经验。其他平台预计也能运行，但磁盘识别、SMART 访问或平台特定功能可能在特殊情况下出现问题。[在此反馈问题。](https://github.com/mcdays94/nas-doctor/issues)

<a id="a-note-from-the-maintainer"></a>

### 维护者的话

NAS Doctor 由一人维护，我每天能亲自操作的平台只有 **Unraid**。群晖 DSM、TrueNAS SCALE、Proxmox VE、Kubernetes／k3s 和 Linux 上的 Docker 均受到支持，并通过模拟、已采集快照的回放，以及少量社区远程实例进行测试。但有些问题在我能直接操作的机器上很容易发现，在其他平台上往往只有用户反馈后才会暴露。

**如果你在群晖、TrueNAS、Proxmox、Kubernetes 或其他非 Unraid 主机上运行 NAS Doctor，发现任何异常，请[提交 Issue](https://github.com/mcdays94/nas-doctor/issues/new/choose)。** 即使是很细微的用户体验问题，也能提供帮助，例如“合并硬盘视图把我的存储卷列了两次”“Proxmox 容器组件显示的是 VM，而不是 LXC”“apcupsd 正在运行，但 UPS 区域是空的”。你们的反馈让这个项目在各个平台上的说明与实际行为保持一致；这些反馈值得感谢，绝不是负担。

---

<a id="file-structure--data-locations"></a>

## 文件结构与数据位置

<a id="inside-the-container-data-volume"></a>

### 容器内（`/data` 卷）

```
/data/
├── nas-doctor.db          # SQLite 数据库（快照、告警、历史、设置）
└── backups/               # 自动数据库备份，可配置
    ├── nas-doctor-2026-04-10.db
    └── ...
```

所有配置均存储在 SQLite 数据库中，通过 `/settings` 网页界面管理，无需手动编辑配置文件。

<a id="host-bind-mounts-read-only"></a>

### 宿主机绑定挂载（只读）

下面汇总所有绑定挂载，与上方各平台的快速开始配置对应。只挂载适用于你所用平台的路径；除 `/data` 外，其余均为只读（RO）。

| 容器路径 | 宿主机路径 | 用途 |
|---|---|---|
| `/host/mnt` | `/mnt` | 磁盘空间监控（Unraid、TrueNAS、Proxmox） |
| `/host/volume<N>` | `/volume<N>` | 磁盘空间监控（群晖，需逐个挂载存储卷） |
| `/host/log` | `/var/log` | 系统日志分析（dmesg、syslog） |
| `/host/boot` | `/boot` | 奇偶校验日志和 Unraid 识别（仅 Unraid） |
| `/etc/unraid-version` | `/etc/unraid-version` | Unraid 系统识别和更新检查（仅 Unraid） |
| `/var/local/emhttp` | `/var/local/emhttp` | 合并硬盘视图中的 Unraid 槽位映射（仅 Unraid） |
| `/dev` | `/dev` | SMART 和 GPU 设备访问 |
| `/sys` | `/sys` | GPU 遥测和硬盘映射 |
| `/var/run/docker.sock` | `/var/run/docker.sock` | 容器监控，自动检测 Docker |
| `/var/run/tailscale` | `/var/run/tailscale` | Tailscale 对等节点拓扑，仅在宿主机上使用 Tailscale 时挂载 |

> **外部 Borg 仓库**需要在上表之外添加相应的绑定挂载。仓库路径的挂载约定和所需环境变量见上方[外部 Borg 监控（宿主机管理的仓库）](#external-borg-monitoring-host-managed-repos)。
>
> **Duplicacy 条目** 同样需要添加绑定挂载，每个仓库或缓存根目录对应一个挂载，并设为**只读**；直接读取磁盘文件的方式可以安全使用只读挂载。参见上方 [Duplicacy 监控（读取磁盘文件，无需可执行文件）](#duplicacy-monitoring-disk-read-no-binary-required)。**无需环境变量**、可执行文件挂载或额外的 Docker 能力。

<a id="source-tree"></a>

### 源码目录

```
cmd/nas-doctor/            # 入口、CLI 参数、演示模式
internal/
├── analyzer/              # 诊断规则引擎、Backblaze 阈值
├── api/                   # HTTP 处理程序、内嵌 HTML 模板、共享 CSS
│   └── templates/         # 仪表盘主题（midnight、clean）和子页面
├── collector/             # 数据采集（SMART、磁盘、Docker、网络、UPS、隧道、传感器）
├── demo/                  # 为演示模式生成模拟数据
├── fleet/                 # 多服务器轮询
├── livetest/              # 进行中测速的广播注册表（SSE 多路分发与回放）
├── logfwd/                # 日志转发（Loki、HTTP JSON、syslog）
├── notifier/              # Webhook 投递和 Prometheus 导出器
├── scheduler/             # 扫描调度、通知规则、服务检查
└── storage/               # SQLite 数据库层
```

---

<a id="resource-usage"></a>

## 资源占用

NAS Doctor 的设计目标是尽量不影响系统运行：

| 资源 | 扫描期间（每 30 分钟一次，每次约 15 秒） | 扫描间隔期间 |
|---|---|---|
| **CPU** | <2% | 约 0% |
| **内存** | 约 30–50 MB | 约 30–50 MB |
| **磁盘 I/O** | 只读：`/proc`、`smartctl`、`dmesg` | 无 |
| **网络** | 操作系统更新检查，每天 1 次请求 | 仅在访问时提供网页界面 |

---

<a id="diagnostic-report"></a>

## 诊断报告

点击仪表盘上的 **Export Report（导出报告）**，生成适合打印的诊断报告。在浏览器中打开后，选择“打印 → 另存为 PDF”。[查看示例报告（PDF）。](docs/nas-doctor-demo-report.pdf)

报告包含 16 个部分：系统概况、诊断结果、硬盘健康与 SMART、Docker、GPU、备份、测速、ZFS、UPS、网络、服务检查、Proxmox、Kubernetes、隧道、奇偶校验和建议操作。

<p>
  <img src="screenshots/report-cover.jpg" alt="报告：封面" width="240">
  <img src="screenshots/report-findings.jpg" alt="报告：诊断结果" width="240">
  <img src="screenshots/report-drives.jpg" alt="报告：硬盘" width="240">
  <img src="screenshots/report-actions.jpg" alt="报告：建议操作" width="240">
</p>

---

<a id="license"></a>

## 许可证

MIT
