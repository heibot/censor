# Censor - 强大的业务审核系统

[![Go Reference](https://pkg.go.dev/badge/github.com/heibot/censor.svg)](https://pkg.go.dev/github.com/heibot/censor)
[![Go Report Card](https://goreportcard.com/badge/github.com/heibot/censor)](https://goreportcard.com/report/github.com/heibot/censor)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

Censor 是一个功能强大的内容审核 Go 组件库，支持多云厂商（阿里云、华为云、腾讯云）、多种内容类型（文字、图片、视频）、灵活的审核策略和业务回调机制。

## 特性

- **多云厂商支持**: 阿里云、华为云、腾讯云内容审核 API 集成
- **多内容类型**: 文字（同步）、图片（同步/异步）、视频（异步）
- **智能文本合并**: 多段文本合并审核，节省 API 调用次数
- **多厂商串联**: 先阿里后华为，可配置触发条件和合并策略
- **人工审核对接**: 统一的人审接口，支持工单系统集成
- **违规内容留存**: 完整的违规证据保存，支持申诉和审计
- **业务状态回调**: Hook 机制驱动业务状态变更，无需硬编码
- **多数据库支持**: MySQL、PostgreSQL、TiDB、ScyllaDB
- **可见性策略**: 灵活的内容展示策略（全部通过/部分允许/创作者可见）

## 安装

```bash
go get github.com/heibot/censor
```

## 快速开始

### 1. 初始化数据库

选择适合你的数据库，执行对应的 SQL 文件：

```bash
# MySQL
mysql -u root -p your_database < store/migrations/mysql.sql

# PostgreSQL
psql -U postgres -d your_database -f store/migrations/postgres.sql

# TiDB
mysql -h tidb-host -P 4000 -u root -D your_database < store/migrations/tidb.sql

# ScyllaDB
cqlsh -f store/migrations/scylla.cql
```

### 2. 创建 Censor 客户端

```go
package main

import (
    "context"
    "database/sql"
    "log"

    censor "github.com/heibot/censor"
    "github.com/heibot/censor/client"
    "github.com/heibot/censor/hooks"
    "github.com/heibot/censor/providers"
    "github.com/heibot/censor/providers/aliyun"
    "github.com/heibot/censor/providers/huawei"
    sqlstore "github.com/heibot/censor/store/sql"

    _ "github.com/go-sql-driver/mysql"
)

func main() {
    // 连接数据库
    db, _ := sql.Open("mysql", "user:pass@tcp(localhost:3306)/censor")
    store := sqlstore.NewWithDB(db, sqlstore.DialectMySQL)

    // 初始化云厂商
    ali := aliyun.New(aliyun.Config{
        ProviderConfig: providers.ProviderConfig{
            AccessKeyID:     "your-key",
            AccessKeySecret: "your-secret",
        },
    })

    hw := huawei.New(huawei.DefaultConfig())

    // 实现业务回调
    myHooks := hooks.FuncHooks{
        OnBizDecisionChangedFunc: func(ctx context.Context, e hooks.BizDecisionChangedEvent) error {
            // 在这里更新你的业务状态
            switch e.Outcome.Decision {
            case censor.DecisionPass:
                // 发布内容
            case censor.DecisionBlock:
                // 隐藏或替换内容
            case censor.DecisionReview:
                // 进入人工审核队列
            }
            return nil
        },
    }

    // 创建客户端
    cli, _ := client.New(client.Options{
        Store:     store,
        Hooks:     myHooks,
        Providers: []providers.Provider{ali, hw},
        Pipeline: client.PipelineConfig{
            Primary:   "aliyun",
            Secondary: "huawei",
            Trigger:   client.DefaultTriggerRule(),
            Merge:     client.MergeMostStrict,
        },
    })

    // 提交审核
    result, _ := cli.Submit(context.Background(), client.SubmitInput{
        Biz: censor.BizContext{
            BizType: censor.BizNoteBody,
            BizID:   "note_123",
            Field:   "body",
        },
        Resources: []censor.Resource{
            {ResourceID: "r1", Type: censor.ResourceText, ContentText: "内容..."},
            {ResourceID: "r2", Type: censor.ResourceImage, ContentURL: "https://..."},
        },
    })

    log.Printf("Review ID: %s", result.BizReviewID)
}
```

## 架构设计

```
┌─────────────────────────────────────────────────────────────────┐
│                         业务系统                                  │
│  (用户服务、笔记服务、聊天服务...)                                 │
└─────────────────────────────────────────────────────────────────┘
                              │ Submit
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│                      Censor Client                               │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐              │
│  │  Pipeline   │  │ TextMerge   │  │   Dedup     │              │
│  └─────────────┘  └─────────────┘  └─────────────┘              │
└─────────────────────────────────────────────────────────────────┘
         │                    │                    │
         ▼                    ▼                    ▼
┌─────────────┐      ┌─────────────┐      ┌─────────────┐
│   Aliyun    │      │   Huawei    │      │  Tencent    │
│  Provider   │      │  Provider   │      │  Provider   │
└─────────────┘      └─────────────┘      └─────────────┘
         │                    │                    │
         └────────────────────┼────────────────────┘
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│                    Violation Translator                          │
│  (统一违规语义层 - 将厂商标签转换为内部标准)                        │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│                         Hooks                                    │
│  OnBizDecisionChanged  │  OnViolationDetected  │  ...           │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│                         Store                                    │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐        │
│  │ MySQL    │  │ Postgres │  │  TiDB    │  │ ScyllaDB │        │
│  └──────────┘  └──────────┘  └──────────┘  └──────────┘        │
└─────────────────────────────────────────────────────────────────┘
```

## 核心概念

### 资源类型 (ResourceType)

| 类型 | 说明 | 同步支持 | 异步支持 |
|------|------|----------|----------|
| `text` | 文本内容 | ✓ | ✓ |
| `image` | 图片 | ✓ | ✓ |
| `video` | 视频 | ✗ | ✓ |

### 业务类型 (BizType)

```go
BizUserAvatar   // 用户头像
BizUserNickname // 用户昵称
BizUserBio      // 用户简介
BizNoteTitle    // 笔记标题
BizNoteBody     // 笔记正文
BizNoteImages   // 笔记图片
BizNoteVideos   // 笔记视频
BizChatMessage  // 聊天消息
BizDanmaku      // 弹幕
// ...
```

### 审核决策 (Decision)

| 决策 | 说明 | 建议处理 |
|------|------|----------|
| `pass` | 通过 | 正常展示 |
| `review` | 需人审 | 暂不展示或创作者可见 |
| `block` | 拦截 | 隐藏或替换 |
| `error` | 错误 | 重试或人工介入 |

### 替换策略 (ReplacePolicy)

| 策略 | 说明 |
|------|------|
| `none` | 不替换，直接隐藏 |
| `default_value` | 使用默认值替换 |
| `mask` | 打码处理 |

## 多厂商串联

```go
Pipeline: client.PipelineConfig{
    Primary:   "aliyun",   // 主审核厂商
    Secondary: "huawei",   // 二次审核厂商
    Trigger: client.TriggerRule{
        OnDecisions: map[censor.Decision]bool{
            censor.DecisionBlock:  true,  // 阿里拦截时，触发华为
            censor.DecisionReview: true,  // 阿里人审时，触发华为
        },
    },
    Merge: client.MergeMostStrict, // 取最严格的结果
}
```

### 合并策略

| 策略 | 说明 |
|------|------|
| `most_strict` | 取最严格结果 (block > review > pass) |
| `majority` | 多数表决 |
| `any` | 任一拦截即拦截 |
| `all` | 全部拦截才拦截 |

## 文本合并优化

```go
// 多段文本合并审核，通过则全部通过，拦截再拆分定位
cli.Submit(ctx, client.SubmitInput{
    Biz: biz,
    Resources: []censor.Resource{
        {ResourceID: "title", Type: censor.ResourceText, ContentText: "标题"},
        {ResourceID: "body",  Type: censor.ResourceText, ContentText: "正文"},
        {ResourceID: "tag1",  Type: censor.ResourceText, ContentText: "标签1"},
    },
    EnableTextMerge: true, // 开启合并
})
```

## 可见性策略

```go
import "github.com/heibot/censor/visibility"

renderer := visibility.NewRenderer()

// 渲染用户资料
result := renderer.RenderUserProfile(
    visibility.ViewerPublic,  // 查看者角色
    "viewer_id",
    "user_id",
    "原始昵称",
    "原始简介",
    "avatar_url",
    bindings, // 从数据库获取的绑定状态
)

if result.Visible {
    for field, rendered := range result.Fields {
        if rendered.IsReplaced {
            // 使用替换后的值
            display(rendered.Value)
        } else {
            display(rendered.Value)
        }
    }
}
```

### 可见性策略类型

| 策略 | 说明 |
|------|------|
| `all_or_nothing` | 任一不通过，整体不可见 |
| `partial_allowed` | 部分不通过，其他仍可见 |
| `creator_only_during_review` | 审核中仅创作者可见 |
| `always_visible` | 始终可见（使用替换值） |

## 处理异步回调

```go
// HTTP 处理器
func handleAliyunCallback(w http.ResponseWriter, r *http.Request) {
    body, _ := io.ReadAll(r.Body)
    headers := make(map[string]string)
    for k, v := range r.Header {
        headers[k] = v[0]
    }

    err := censorClient.HandleCallback(r.Context(), "aliyun", headers, body)
    if err != nil {
        http.Error(w, err.Error(), http.StatusBadRequest)
        return
    }
    w.WriteHeader(http.StatusOK)
}
```

## 统一违规语义

Censor 提供统一的违规语义层，将不同厂商的标签转换为内部标准：

```go
import "github.com/heibot/censor/violation"

// 违规领域
violation.DomainPornography  // 色情
violation.DomainViolence     // 暴力
violation.DomainPolitics     // 政治
violation.DomainSpam         // 垃圾信息
// ...

// 违规标签
violation.TagNudity          // 裸露
violation.TagHateRace        // 种族仇恨
violation.TagSpamAds         // 广告
// ...
```

## 目录结构

```
censor/
├── consts.go           # 常量定义
├── types.go            # 核心类型
├── errors.go           # 错误定义
├── client/             # 客户端
│   ├── client.go       # 主客户端
│   ├── options.go      # 配置选项
│   └── pipeline.go     # 审核流水线
├── providers/          # 云厂商适配
│   ├── provider.go     # 接口定义
│   ├── aliyun/         # 阿里云
│   ├── huawei/         # 华为云
│   ├── tencent/        # 腾讯云
│   └── manual/         # 人工审核
├── store/              # 数据存储
│   ├── store.go        # 接口定义
│   ├── sql/            # SQL 实现
│   └── migrations/     # 数据库脚本
├── hooks/              # 业务回调
│   ├── hooks.go        # 接口定义
│   └── event.go        # 事件类型
├── violation/          # 违规语义
│   ├── domain.go       # 违规领域
│   ├── tag.go          # 违规标签
│   ├── unified.go      # 统一模型
│   └── translator.go   # 翻译器
├── visibility/         # 可见性
│   ├── policy.go       # 策略定义
│   └── render.go       # 渲染器
├── utils/              # 工具函数
│   ├── hash.go         # 哈希
│   ├── textmerge.go    # 文本合并
│   └── idgen.go        # ID 生成
└── example/            # 使用示例
    └── main.go
```

## 数据库表

| 表名 | 说明 |
|------|------|
| `biz_review` | 业务审核单 |
| `resource_review` | 资源审核记录 |
| `provider_task` | 厂商任务记录 |
| `censor_binding` | 当前绑定状态 |
| `censor_binding_history` | 状态变更历史 |
| `violation_snapshot` | 违规证据快照 |

## 最佳实践

1. **使用 Hook 而非硬编码**: 业务状态变更通过 Hook 实现，保持解耦
2. **启用内容去重**: 相同内容无需重复审核，节省成本
3. **合理配置文本合并**: 短文本合并可显著减少 API 调用
4. **串联多厂商**: 重要内容建议多厂商交叉验证
5. **保留违规证据**: 便于申诉和法务需求
6. **监控异步任务**: 定期轮询未完成的异步任务

## License

MIT License - 详见 [LICENSE](LICENSE)

## 贡献

欢迎提交 Issue 和 Pull Request！

## 支持

- 文档: [https://github.com/heibot/censor](https://github.com/heibot/censor)
- Issues: [https://github.com/heibot/censor/issues](https://github.com/heibot/censor/issues)
