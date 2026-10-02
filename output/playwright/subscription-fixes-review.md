# 订阅后端修复与验证记录

日期：2026-10-01。范围仅为根项目 `model/subscription*.go`、`controller/subscription.go` 及对应 Go 测试；没有修改 `gateway/` 旧副本、RelayFor 前端、统计文件或数据库，也没有启动服务或执行真实支付。

## 已修复

1. **余额购买确认价校验。** `SubscriptionBalancePayRequest` 现在可接收 `expected_price_amount` 和 `expected_currency`。两者都是可选指针；旧客户端不传仍沿用原行为。`PurchaseSubscriptionWithBalance` 在同一事务内重新读取当前套餐行，先检查内部/预付卡属性、启用状态、余额支付权限与用户确认的价格和币种，再计算及扣除钱包额度。价格或币种改变会返回“请刷新后重新确认”，且不会创建订单/订阅或扣款。空币种的历史套餐按 USD 兼容。购买日志改为保留实际价格精度。
2. **重叠 VIP/SVIP 分组回退。** 短期高级套餐到期、管理员终止或删除后，按仍有效且最近开通的升级套餐恢复用户组并刷新缓存；不再因为存在任意有效套餐就保留已结束套餐的高权限组。
3. **内部与预付卡套餐隔离。** 用户套餐目录、管理员通用套餐列表、通用更新和状态接口排除 `RelayForSubscriptionPolicy.Internal=true` 及关联预付卡的计划（包含软删除卡的历史关联）。余额购买事务也拒绝这些计划，即使数据库中误标为启用。四种第三方支付都通过 `SubscriptionOrder.Insert()` 创建待支付订单；该入口现于插入前在事务内检查最新套餐状态和内部/预付卡标记，已知 ID 也不能创建新订单。既有订单的回调完成路径未加新拒绝条件。
4. **新建停用套餐。** 创建请求显式传入 `enabled=false` 时，在创建事务中明确持久化该值，避免 GORM 的 `default:true` 把草稿套餐自动启用；旧请求省略该字段仍默认启用。
5. **订阅摘要。** `/api/subscription/self` 的活动与历史两组摘要都附带 `plan_title` 和 `plan_internal`。名称从实际套餐表批量查询，停用套餐仍可显示；`plan_internal` 综合内部策略、预付卡关联（包含软删除历史）及预付卡资金来源判定。原有订阅/资金记录仍完整返回；旧库缺少扩展表时也可返回摘要。

## 回归测试

- **已通过，首次定向模型测试：** `go test ./model -run '^TestBalancePurchase' -count=1`，覆盖缓存中旧价、事务新价、币种不符、显式零价、旧客户端兼容，以及内部/软删除预付卡计划拒买。之后追加的空币种历史套餐兼容用例尚未跑通，原因是下述并行编译阻断。
- 新增 `TestPendingThirdPartySubscriptionOrdersRejectProtectedPlanIDs`，覆盖 Stripe、Creem、易支付和 Waffo Pancake 的内部计划，以及历史软删除预付卡计划；**当前尚未跑通**，原因是下述并行编译阻断。
- **已通过，分组回退测试：** `TestExpireDueSubscriptionsRestoresSurvivingPlanGroup`、`TestAdminInvalidateSubscriptionRestoresSurvivingPlanGroup`、`TestAdminDeleteSubscriptionRestoresSurvivingPlanGroup`；连同现有鉴权和缓存相关测试共 5 个通过。
- **已通过，摘要测试：** `TestGetAllUserSubscriptionsPreservesPlanTitlesAndMarksInternalFunding` 与 `TestSubscriptionSummariesWorkBeforeRelayForMetadataTablesExist`；相关订阅模型测试通过。
- **已通过，通用套餐接口测试（并行源码编译正常时）：** `TestSubscriptionPlanCatalogExcludesInternalAndPrepaid`、`TestAdminSubscriptionPlanMutationRejectsInternalAndPrepaid`、`TestAdminCreateSubscriptionPlanPersistsExplicitDisabled`。
- 新增 `TestSubscriptionBalancePayPassesExpectedPriceAndCurrency`，检查 HTTP 请求字段确实进入购买校验；**当前尚未跑通该项**，见下述整合阻断。

## 当前整合阻断与剩余风险

- 最新一次 `go test ./controller` 被本次编辑范围外的 `controller/user.go:383` 编译错误阻断：调用 `time.Now()` 但当前文件未导入 `time`。最新一次 `go test ./model` 又被并行新增的 `model/relayfor_activities.go:57` 常量乘法溢出阻断。这些文件没有在本任务中修改；整合构建须在所属任务修复后复跑。本任务曾尝试用临时 Go overlay 隔离无关编译错误，自动审批策略拒绝了该命令，未执行覆盖测试。
- `controller/relayfor_subscriptions.go:59` 的原生 RelayFor 套餐状态入口仍可按已知 ID 更新启用状态；本次限定只能改 `controller/subscription.go`，因此尚未在该入口加相同保护。
- 第三方支付控制器在调用 `SubscriptionOrder.Insert()` 之前仍使用缓存套餐进行预检查，并且 Stripe 会先建立外部 checkout session。最终待支付订单有模型层保护，但这些控制器若要更早返回明确业务错误，应在各入口直接调用同一“可售计划”校验；这些文件在本次编辑范围之外。

## 主要修改文件

- `model/subscription.go`、`model/subscription_price_expectation_test.go`、`model/subscription_group_fallback_test.go`、`model/subscription_summary_test.go`
- `controller/subscription.go`、`controller/subscription_plan_guard_test.go`、`controller/subscription_balance_quote_test.go`
