# further-api 常量与运行参数

`constant/` 汇总网关使用的协议类型、渠道编号、上下文键、任务动作和运行参数。这里只保存这些定义与数据约定；账户、路由、认证、数据库事务和网络执行由对应服务负责。

| 文件 | 内容 |
|---|---|
| `api_type.go` | 各上游 API 类型编号。 |
| `channel.go` | 渠道编号、默认厂商地址及厂商能力目录。 |
| `endpoint_type.go` | 接口类型定义。 |
| `azure.go` | Azure API 和历史部署名称兼容所需的常量。 |
| `cache_key.go` | 缓存键与字段命名。 |
| `context_key.go` | 请求中的账户、令牌、渠道与执行状态键。 |
| `env.go` | 在初始化时赋值的运行参数，包括超时、大小限制和日志设置。 |
| `finish_reason.go` | 标准结束原因。 |
| `midjourney.go` | MJ 动作与协议错误定义。 |
| `multi_key_mode.go` | 渠道多密钥选择模式。 |
| `setup.go` | 初始化状态。 |
| `task.go` | 任务平台、动作及映射定义。 |
| `waffo_pay_method.go` | 支付方式的标识与目录数据。 |

数字编号、协议字符串和迁移约定可能保留历史含义，修改时需要同时核对数据库数据与调用方。厂商名称标识实际上游服务；网关产品名称为 **further-api**。代码来源和许可要求见根目录 [LICENSE](../LICENSE)、[NOTICE](../NOTICE) 与 [第三方许可](../THIRD-PARTY-LICENSES.md)。