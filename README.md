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
        ProjectName: "fuloot-plugin-server",
        PluginMode:  true,
        Version:     "1.0.0",
        StartFunc:   cmd.StartServer,
        InfoFunc:    cmd.GetInfo,
    })
}
```

启动和 `status` Banner 会显示项目名称及插件模式状态：

```text
project name             fuloot-plugin-server
plugin mode              enabled
```

`ProjectName` 留空时默认显示 `FulootAPI`；`PluginMode` 默认值为 `false`，Banner 显示 `disabled`。

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

> **设计原则**：`start` 默认前台运行，便于 air 等热重载工具管理进程生命周期；加 `-d` 才以守护进程方式后台运行，用于生产部署。

## 开发与发布流程

fuloot-cli 作为外部依赖被 ft_go 等项目引用。修改后需遵循以下流程：

```bash
# 1. 在 fuloot-cli 仓库修改代码
cd /data/apps/goProject/fuloot-cli

# 2. 提交并打 tag
git add .
git commit -m "feat: description of change"
git tag v0.x.x
git push origin main --tags

# 3. 在引用项目中更新依赖
cd /data/apps/goProject/ft_go
go get github.com/laputacn/fuloot-cli@latest
go mod tidy
```

如果需要在发布前本地测试，可在引用项目的 `go.mod` 中临时添加 replace 指令：

```
replace github.com/laputacn/fuloot-cli => /data/apps/goProject/fuloot-cli
```

测试完成后删除 replace 行，再通过正式 tag 引用。

## License

MIT
