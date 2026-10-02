# RelayFor 原前端恢复源码

来源：`C:/Users/Administrator/Desktop/relayfor-rebuild-summary`。原始哈希、归档位置和资源闭包见 `source-provenance.json`。

最初恢复首页、登录、用户控制台、管理员页面的原 DOM 与布局；随后按正式网关需求修改了首页、控制台、管理员页面及样式。`source-provenance.json` 中的哈希指向恢复时的原始文件，不是当前工作源码的哈希。共享资源使用 `/relayfor-assets/`，管理员专用脚本位于该命名空间的 `admin/`。

当前 `relayfor-ui/` 是正式原桌面界面的工作源码。HTML 中标记为 `application/x-relayfor-legacy` 的脚本由 `web/src/relayfor/bootstrap.ts` 在会话与站点检查后加载，业务请求经真实 New API 适配。原始文件完整归档于项目 `.agent-work/relayfor-original-source`，不得发布此归档目录。

没有引入旧 fixture、旧预览服务、演示认证或缓存 API 响应。当前预付卡页面连接真实网关接口；旧商业双站内容已从工作界面移除。支付与订阅相关的 New API 实现按实际业务用途保留。
