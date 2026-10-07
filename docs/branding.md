# further-api 品牌与资源

产品名称在所有语言中统一为 **further-api**。项目仓库为 [dqt081228OHVGFY/further-api](https://github.com/dqt081228OHVGFY/further-api)，使用说明从 [文档目录](./README.md) 开始。

## Logo 与图标

正式 Logo 恢复为原来的蓝底白色握手图案，原始像素保存在 [`web/public/further-api-logo.png`](../web/public/further-api-logo.png)。[`SVG 入口`](../web/public/further-api-logo.svg) 嵌入同一原图，访问路径为 `/further-api-logo.svg?v=handshake-20261004`；版本参数让旧浏览器及时读取恢复后的标志。[`favicon.ico`](../web/public/favicon.ico) 由原 PNG 生成，包含 16、32、48、64、128、256 像素六个尺寸。

`web/public/logo.png` 兼容历史资源路径，显示同一握手图案；新引用使用正式路径。启动迁移将旧站点 Logo 配置统一到正式 SVG 地址。产品文字名称保持 further-api。

支付服务图标 `waffo-logo-*.svg` 表示对应的第三方服务，不作为产品 Logo。

移动菜单的默认账户头像位于 `web/public/avatars/01.png`，由同目录 `01.svg` 生成；该图形仅代表账户头像。

## 翻译与署名

产品名称、页面文本和资源统一使用 further-api。翻译中的插值占位符、HTML 标签与真实协议标识保持有效；技术常量和第三方组件名称按其实际含义使用。历史标识仅在明确的兼容读取、数据迁移或归属说明中保留，不作为可切换的网关品牌。

法律文件和界面法律署名保留原作者信息与 `NOTICE` 要求的原项目链接。界面使用的 Lucide 图标按 [第三方许可](../THIRD-PARTY-LICENSES.md) 提供；品牌变更不改变原作品的许可要求。

后端核心执行流程与控制台已作进一步重写，范围见 [后端实现与迁移](./backend-architecture.md)。官方 DTO、SDK、协议规则和历史贡献继续构成项目的一部分；产品采用 further-api 品牌不表示第三方代码全部消失或每一行代码均为新作。发行时保留 `LICENSE`、`NOTICE` 和第三方许可文件。
