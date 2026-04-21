# 账号链式兜底设计

## 背景

当前仓库已经具备“账号失败后切换到其他账号”的通用 failover 能力，但行为是从当前分组的可调度账号集合中重新挑选，不支持为单个账号显式指定固定兜底链。目标是在保留现有调度与分组隔离规则的前提下，增加账号级链式兜底能力：

- 管理员可为账号配置 `A -> B -> C` 形式的兜底链
- 仅允许同平台账号互相兜底
- 任意上游错误进入 failover 时，都优先尝试链式兜底
- 兜底链耗尽后，再回退到现有普通 failover

## 决策

采用 `accounts.fallback_account_id` 正式字段，而不是存入 `extra`：

- 需要强类型校验和循环检测
- 需要在删除目标账号时自动置空
- 需要在前后端稳定暴露并支持后续扩展

本次不引入独立规则表，也不做按错误码分流；保持“一条默认下一跳”模型。

## 数据模型

新增数据库字段：

- `accounts.fallback_account_id BIGINT NULL REFERENCES accounts(id) ON DELETE SET NULL`

同步扩展这些层：

- `service.Account`
- 账号仓储读写与列表查询
- admin handler 的创建/编辑请求
- DTO 与前端 `Account` / `CreateAccountRequest` / `UpdateAccountRequest`

## 保存时校验

创建/编辑账号时新增统一校验：

1. `fallback_account_id` 为空时直接通过
2. 不能指向自己
3. 目标账号必须存在
4. 目标账号平台必须与当前账号一致
5. 检测循环引用，拒绝 `A -> B -> C -> A`

不强制要求兜底账号与当前账号处于完全相同的分组。因为一个账号可能属于多个分组，保存时做强绑定会误伤合法配置。实际是否可用改为在请求运行时按当前分组与模型再次过滤。

## 运行时行为

不重写现有 failover loop，只增强“切换候选账号来源”。

### 触发时机

当转发返回 `UpstreamFailoverError` 时，无论状态码是 `4xx`、`429`、`5xx`、超时还是网络错误，都允许进入链式兜底判断。现有“同账号重试”语义保持不变，仍优先于切换账号。

### 候选顺序

当某账号需要切换时：

1. 先读取当前失败账号的 `fallback_account_id`
2. 沿链向后遍历，直到找到第一个可用账号
3. 若整条链都不可用，再退回现有普通 failover

### 链上账号的可用条件

链上的每一跳都必须同时满足：

- 未出现在本次请求的失败/排除列表中
- 平台与失败账号一致
- 在当前请求作用域内可见
  - 当前 group 下请求：必须属于当前 group
  - 无 group 请求：必须符合现有 ungrouped 规则
- 当前可调度，未被限流、过载、临时封禁
- 支持当前请求模型
- 不违反现有 channel restriction / upstream restriction

命中链式兜底账号后：

- 更新当前请求的失败列表，避免再次回到已失败账号
- 复用现有粘性会话更新逻辑
- 复用现有 cache billing / account switch / usage 记录逻辑

### 边界

- 如果响应已经开始向客户端写出，不再进行链式兜底切换
- 若链条中某个账号已删除，依赖外键 `ON DELETE SET NULL` 自动断链
- 若链条中某个账号当前不在本分组、模型不兼容或不可调度，直接跳过到下一跳

## 前端

在账号创建与编辑弹窗中新增“兜底账号”选择器：

- 仅展示同平台账号
- 允许为空
- 编辑时回显当前配置

本次不在批量编辑中暴露该字段，避免一次操作产生错误链配置。

## 受影响模块

- `backend/migrations`
- `backend/internal/service/admin_service.go`
- `backend/internal/service/account.go`
- `backend/internal/repository/account_repo.go`
- `backend/internal/handler/admin/account_handler.go`
- `backend/internal/handler/dto/*`
- `backend/internal/handler/failover_loop.go` 及调用方
- `backend/internal/service/gateway_service.go`
- `backend/internal/service/openai_gateway_service.go`
- `frontend/src/types/index.ts`
- 账号创建/编辑相关 Vue 组件

## 测试

至少覆盖以下场景：

1. 保存校验
   - 自引用失败
   - 跨平台失败
   - 环形链失败
2. 运行时链式兜底
   - `A -> B -> C`，A 失败后命中 B
   - B 不可调度时跳到 C
   - B 不在当前分组时跳过
   - B 不支持模型时跳过
3. 回退逻辑
   - 链耗尽后继续走现有普通 failover
4. 删除语义
   - 被引用账号删除后，原账号 `fallback_account_id` 自动清空

## 非目标

- 不做跨平台兜底
- 不做按错误码路由到不同兜底账号
- 不做 fallback 黑名单或失败计数器
- 不修改现有调度优先级与负载算法，只在 failover 切换路径上插入链式优先级

## 实施顺序

1. 数据库 migration 与模型字段打通
2. admin create/update 校验与接口透出
3. failover 选择逻辑增加链式候选
4. 前端创建/编辑弹窗接入
5. 单元测试与集成测试补齐
