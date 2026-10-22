# fuloot-cli

FulootAPI 的 CLI 脚手架库，提供进程管理（start/stop/restart/status）、Banner 显示和版本管理。

## 安装

```bash
go get github.com/laputacn/fuloot-cli@latest
```

## 使用

```go
package main

import (
    "yourapp/cmd"
    fulootcli "github.com/laputacn/fuloot-cli"
)

func main() {
    fulootcli.Run(fulootcli.App{
        ProjectName:   "FulootAPI",
        FulootVersion: fulootcli.FulootV1Version,
        AppName:       "RYS Movie",
        AppVersion:    cmd.Version,
        PluginMode:    false,
        StartFunc:   cmd.StartServer,
        InfoFunc:    cmd.GetInfo,
    })
}
```

启动和 `status` Banner 会显示项目名称及插件模式状态：

```text
project name             FulootAPI
app name                 RYS Movie
plugin mode              disabled
fuloot version           1.0.0
app version              1.0.20
```

`ProjectName` 表示产品（API 为 `FulootAPI`，插件平台/插件为 `FulootPlug`），
`AppName` 表示具体业务名称。`FulootVersion` 选用 CLI 中的 `FulootV1Version`
或 `FulootV2Version`；当前分别为 `1.0.0`、`2.0.0`。带插件基座的 petmini
使用 V2，旧宠物、影视、旅游分支使用 V1；关闭插件模式不会把 V2 变成 V1。
以后系统版本的补丁更新只需修改 CLI 的 `identity.go` 并推送，项目重新 build 即可。

`AppVersion` 是业务构建版本，继续由 `build.sh -v` 注入 `cmd.Version`。
旧 `Version` 字段仍作为 `AppVersion` 的兼容回退；`version` 命令始终输出应用版本。
未指定系统版本时默认 V1，未指定名称时默认 `FulootAPI`，未指定应用版本时显示 `dev`。
`PluginMode` 默认值为 `false`，Banner 显示 `disabled`。

独立插件进程从编译时嵌入的 `manifest.json` 读取名称与版本：

```go
info, err := fulootcli.PluginInfoFromManifest(pluginmeta.ManifestJSON, version)
if err != nil { log.Fatal(err) }
version = info.Version // 构建注入优先；未注入时使用 manifest 版本
fulootcli.PrintPluginBanner(info, socket)
```

插件横幅显示 `project name = FulootPlug`、`fuloot version = 2.0.0`，
`app name` 为插件 ID，`app version` 为构建版本，另有 `plugin name`（清单名称）
及 `plugin version`（打包清单的版本）。插件通过 gRPC 运行，显示真实监听 endpoint，
不增加 HTTP/daemon 命令。使用通用 `Run` 的插件也可以设置 `App.PluginName` 和
`App.PluginVersion`。插件运行时/市场服务本身显示应用信息，已安装插件列表仍逐项显示名称和版本。

## 命令说明

| 命令 | 说明 |
|------|------|
| `./app` | 前台运行（默认） |
| `./app start` | 前台运行（默认） |
| `./app start -d` | 以 daemon 后台方式运行 |
| `./app start --mode release` | 指定运行模式 |
| `./app stop` | 停止后台进程 |
| `./app restart` | 重启后台服务 |
| `./app status` | 查看进程状态 |
| `./app plugins` | 显示插件模块信息及本机已安装插件列表（应用配置 `PluginsFunc` 后启用） |
| `./app version` | 打印版本号 |
| `./app build-info` | 打印当前二进制的 Go 版本、依赖和构建参数（JSON） |

> **设计原则**：`start` 默认前台运行，便于 air 等热重载工具管理进程生命周期；加 `-d` 才以守护进程方式后台运行，用于生产部署。

## 线上 CLI 驱动业务构建

接入 `scripts/fuloot-go.sh` 的业务项目，正常执行 `./build.sh` 即可同步线上
CLI。默认跟随 `https://github.com/laputacn/fuloot-cli.git` 的 `main` 分支，
不需要维护本地 CLI 仓库，也不需要每次手动 `go get`。

每次构建依次执行：

1. 获取 `main`，固定本次使用的完整 Git commit。
2. 从这个提交读取 `.fuloot-toolchain` 并加载中央构建脚本。
3. 自动下载并选择指定的**精确 Go 版本**。
4. 将业务 `go.mod` / `go.sum` 的 CLI 依赖更新到同一提交，保留其他本地 replace。
5. 编译业务程序，并检查产物真实的 Go 版本和已链接的 CLI 版本。

本次解析的仓库、分支、commit、CLI module version、Go 版本记录在业务项目的
`.fuloot-build.json`。同步也更新 `.fuloot-toolchain` 和已有 Dockerfile 的 Go 标签；
这些变化可以随业务版本提交，便于审计和复现。依赖解析使用临时 modfile，下载或
解析失败不会覆盖原来的项目配置。线上 CLI 的 replace 会被拒绝，避免误用本地代码。

启动横幅的 `go version` 始终来自 `runtime.Version()`，显示真实编译器版本。
`fuloot version` 是中央系统版本；`app version` 是业务版本，由业务 `build.sh -v=...` 注入。
CLI 提供的 start/stop/restart/status、横幅和通用命令会随重编译更新；需要新增业务
回调、配置或服务接口的功能，仍须在对应业务实现接入。

### 日常更新

修改 CLI 的启动功能或横幅后，提交并推送到 `main`。接入的业务项目下次正常执行
`./build.sh` 就会同步。**只有本地 commit 不够，必须 push 到 GitHub。**
默认使用分支提交，无需额外打 tag；`@latest` 仍主要用于手动安装发布版，不能代替
这里的跟随 `main` 流程。

### 升级 Go

在 CLI 仓库中执行（下例版本仅为流程示意，使用已正式发布且验证过的版本）：

```bash
cd /data/apps/goProject/fuloot-cli
sh build.sh
./dist/fuloot-dev toolchain set 1.27.10
# 审查并提交 .fuloot-toolchain、go.mod、fuloot-toolchain.json 等变更，然后 push main
```

不带 `--all` 时只更新 CLI 仓库，无需其他业务 checkout。CLI 自身的 `build.sh`
使用本地策略，允许先测试尚未推送的修改。业务项目之后执行：

```bash
cd /path/to/business
./build.sh -v=YOUR_BUSINESS_VERSION
```

该构建使用线上 CLI 的 Go 版本；不要求预先替换机器的 `/usr/local/go`。
机器需有 Git 和 Go 1.21+，以及访问 GitHub、Go 模块代理和工具链下载的网络。
可以用 `GO_BINARY` 指定已有 Go 的路径。

### 固定版本、离线构建和边界

```bash
# 按已知的完整 CLI commit 重新构建；仍从远端验证并获取该提交
FULOOT_CLI_REF=FULL_COMMIT_SHA ./build.sh

# 显式冻结：使用当前 go.mod / go.sum / .fuloot-toolchain，不同步远端
# 离线时还需要相应工具链和模块已在本机缓存
FULOOT_CLI_SYNC=0 ./build.sh
```

默认联网构建失败时会停止，不会自动退回旧 CLI。`FULOOT_CLI_REPOSITORY` 可显式
指定测试用仓库或镜像；正式环境不设置时始终使用上面的 GitHub 地址。
交叉编译仍使用 `GOOS` / `GOARCH`，构建助手在宿主平台执行。

自动同步的入口是 **`build.sh`**。裸 `go build`、IDE 的构建按钮、直接
`docker build` 都不会主动更新依赖。Docker 需先运行 `build.sh` 同步项目配置，
然后重建镜像；已有镜像和运行中的进程不会随 GitHub 更新自动改变。
重新部署并重启后，新启动横幅才会显示新 Go 版本。

首次启用必须先将本仓库包含 `.fuloot-toolchain`、`scripts/fuloot-build.sh` 和
`cmd/fuloot-dev` 的变更推送到线上，再发布各业务的 bootstrap。若远端缺少这些文件，
构建会提示先发布 CLI。未发布前可用显式冻结模式继续构建原来的项目快照。

## 工作区检查与批量构建

`fuloot-toolchain.json` 保留本地工作区项目清单。路径相对于该文件解析；
从其他目录使用工具时可传 `--config /path/to/fuloot-toolchain.json`，或设置
`FULOOT_TOOLCHAIN_CONFIG`。

```bash
# 额外同步当前工作区全部项目的 Go 声明、脚本和 Docker 标签
./dist/fuloot-dev toolchain set 1.26.10 --all
./dist/fuloot-dev toolchain check --all

# 测试和构建当前已同步的工作区快照；不在测试后切换到另一个线上 CLI
./dist/fuloot-dev test --all
./dist/fuloot-dev build --all
./dist/fuloot-dev build --project movie-rys --version 2.0.1

# 读取二进制构建信息，不执行目标程序
./dist/fuloot-dev toolchain verify /path/to/fuloot
./dist/fuloot-dev toolchain verify --binary bin/plugin-farm-server /path/to/plugin.zip
```

批量命令消费本地锁定配置，等同于显式冻结模式；若要同步线上 CLI，先在业务目录
执行 `build.sh`。`toolchain check` 对比工作区策略，线上版本高于本地 CLI checkout 时
应先更新本地 CLI，再检查。批量构建默认运行 Go 测试和 vet；`--test=false` 只跳过
测试，仍校验最终二进制。新输出目录中的 `build-report.json` 记录产物、SHA-256、
实际 Go、业务版本、测试状态和工作区 Git 状态。不会覆盖已有报告或产物。

服务的默认业务版本是 `dev`，正式构建请明确指定版本；插件默认从 manifest 读取，
分发新版插件时递增补丁版本。V2 Docker 构建上下文仍为其父目录，以包含本地 Runtime：

```bash
docker build --pull -f /data/apps/petmini/api-petmini/Dockerfile \
  -t fuloot-api:YOUR_VERSION /data/apps/petmini
```

## 接入新业务项目

复制本仓库 `scripts/fuloot-go.sh` 到项目同名路径，在现有构建脚本中加入：

```sh
. "$ROOT/scripts/fuloot-go.sh"
fuloot_go_setup "$ROOT"
# 使用 "$GO_CMD" build ... 编译，然后对尚未压缩的二进制执行：
fuloot_verify_go "$OUTPUT"
```

提交这一次接入改动即可。稳定 bootstrap 每次获取线上构建逻辑，后续中央逻辑改进
不要求手工复制脚本。新增批量构建目标时再登记 `fuloot-toolchain.json`。
保持 CLI 导出 API 和 bootstrap 接口兼容；不兼容的 API 变更仍会要求业务代码迁移。

## License

MIT
