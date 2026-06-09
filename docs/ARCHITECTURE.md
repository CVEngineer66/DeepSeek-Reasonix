+=====================================================================================================+
|                                      记忆系统完整架构                                               |
|                              启动加载层 · 运行时层 · 持久化层                                       |
+=====================================================================================================+

  BOOT TIME (一次启动)
  ====================================================================================================
  boot.go  ────  memory.Load() ──────────────────────────────────────────────────────────────────┐
                                                                                                  │
  ┌─────────────────────────────────── memory.Set ───────────────────────────────────────────────┐│
  │                                                                                             ││
  │  ┌──────────── Docs ────────────┐  ┌────────── Index ──────────┐  ┌───────── Store ────────┐ ││
  │  │                             │  │                          │  │                        │ ││
  │  │ user:   ~/.config/.../     │  │ MEMORY.md 文本内容         │  │ Save / Delete / List    │ ││
  │  │         REASONIX.md        │  │ 格式:                      │  │ AlwaysOnFacts           │ ││
  │  │                             │  │ - [title](x.md) — desc    │  │ read_memory             │ ││
  │  │ ancestor:  ../REASONIX.md  │  │   (2026-06-09) 日期        │  │                        │ ││
  │  │                             │  │ 200行上限 · recency排序    │  │ 文件: .md + frontmatter  │ ││
  │  │ project: ./REASONIX.md     │  │                          │  │ 目录: /memory/           │ ││
  │  │         ./AGENTS.md        │  │                          │  │ 主题: /memory/topic/     │ ││
  │  │         ./CLAUDE.md        │  │                          │  │                        │ ││
  │  │                             │  │                          │  │ activation, created_at   │ ││
  │  │ local: ./REASONIX.local.md │  │                          │  │ updated_at              │ ││
  │  │         ./AGENTS.local.md  │  │                          │  │                        │ ││
  │  │         ./CLAUDE.local.md  │  │                          │  │                        │ ││
  │  └─────────────────────────────┘  └──────────────────────────┘  └────────────────────────┘ ││
  │                                                                                             ││
  └─────────────────────────────────────────────────────────────────────────────────────────────┘│
                                                                                                  │
       │                                                                                          │
       ▼                                                                                          │
  ┌────────────────────────────────────────────────────────────────────────────────────────────┐  │
  │  memory.Compose(basePrompt, mem)                                                            │  │
  │                                                                                             │  │
  │  # Memory                                                                               ←┐  │  │
  │    Persistent context loaded from memory files...                                       │  │  │
  │    ## /path/to/REASONIX.md (project)                                                    │  │  │
  │      <doc 全文>                                                                          │  │  │
  │    ## /path/to/REASONIX.local.md (local)                                                 │  │  │
  │      <doc 全文>                                                                          │  │  │
  │    ## Always-on facts                                                                    │  │  │
  │      These facts have activation=always_on, so their full content is already loaded.     │  │  │
  │      You do not need to call read_memory for these.                                      │  │  │
  │      ### prefers-tabs                                                                    │  │  │
  │        Always indent with tabs in this project.                                         │  │  │
  │    ## Saved memories（仅 model_decision）                                              │  │  │
  │      model_decision facts only show their description here; use read_memory for full body.  │  │  │
  │      (主动保存指引: when user corrects you, save it)                                        │  │  │
  │      - [DB Config](db-config.md) — PostgreSQL connection info (2026-06-01)                 │  │  │
  │                                                                                     ────┘  │  │
  │  拼入系统 prompt → agent.NewSession(sysPrompt) → Session.Messages[0]                   │  │  │
  │  ↓                                                                                       │  │  │
  │  前缀缓存命中（整个会话不变）                                                              │  │  │
  └────────────────────────────────────────────────────────────────────────────────────────────┘  │
                                                                                                   │
  ┌─── 启动时 ──────────────────────────────────────────────────────────────────────────────────┘

  +=================================================================================================+

  SESSION TIME (每轮对话)
  ====================================================================================================
  模型调用 remember/forget/read_memory         用户操作                         后台
  ┌──────────────────────────────┐       ┌────────────────────┐          ┌───────────────────┐
  │ rememberTool                 │       │ # 记住 X           │          │ refreshMemory()    │
  │   .Execute()                 │       │   QuickAdd()       │          │ ← SaveDoc          │
  │   store.Save()               │       │   AppendDoc()      │          │ ← QuickAdd         │
  │   QueueMemory()              │       │   QueueMemory()    │          │ ← 工具调用         │
  │                              │       │   refreshMemory()  │          │                   │
  │ forgetTool                   │       │                    │          │ 重新 memory.Load() │
  │   .Execute()                 │       │ 面板编辑            │          │ → c.mem 更新       │
  │   store.Delete()             │       │   SaveDoc()        │          │ → MemoryPanel 刷新  │
  │   QueueMemory()              │       │   WriteDoc()       │          │                   │
  │   refreshMemory()            │       │   DocDiff()        │          │                   │
  │                              │       │   refreshMemory()  │          │                   │
  │ readMemoryTool               │       │                    │          │                   │
  │   .Execute()                 │       │                    │          │                   │
  │   store.List() + 按 name 匹配  │       │                    │          │                   │
  └──────────┬───────────────────┘       └────────┬───────────┘          └───────────────────┘
             │                                   │
             └───────────────┬───────────────────┘
                             │
                             ▼
  ┌───────────────────────────────────────────────────────────────┐
  │  c.pendingMemory = ["Saved memory X", "Memory file updated"]  │
  └───────────────────────────┬───────────────────────────────────┘
                              │
             下一轮 submit()  │
              Controller.Compose(input)
                              │
                              ▼
  ┌───────────────────────────────────────────────────────────────┐
  │  <memory-update>                                              │
  │  The following project-memory changes were just made:         │
  │  - Saved memory "prefers-tabs": User prefers tabs over spaces │
  │  </memory-update>                                             │
  │  <用户输入>                                                   │
  └───────────────────────────┬───────────────────────────────────┘
                              │
                              ▼
  ┌───────────────────────────────────────────────────────────────┐
  │  模型请求:                                                     │
  │  [system]  # Memory ← 缓存的旧索引，字节不变，命中               │
  │  [user]    <memory-update>...  ← 新事实，模型感知变化           │
  │  [user]    用户输入                                            │
  │  ↓                                                             │
  │  前缀缓存命中 ✅  新事实在 user message 中不破坏缓存             │
  └───────────────────────────────────────────────────────────────┘
  +=================================================================================================+

  持久化层
  ====================================================================================================
  ~/.config/reasonix/
  ├── REASONIX.md                            ← 用户级指令文档
  │
  └── projects/
      └── <slug>/
          └── memory/
              ├── MEMORY.md                  ← 索引（200行上限、recency排序）
              │                               每行：- [标题](slug.md) — 描述 (2026-06-09)
              │
              ├── prefers-tabs.md            ← 单文件事实
              │   ---
              │   name: prefers-tabs
              │   title: Prefers tabs
              │   description: User prefers tabs
              │   activation: always_on       ← always_on | model_decision
              │   topic:                     ← 可选，frontend-style
              │   type: user                 ← user | feedback | project | reference
              │   created_at: 2026-06-09T20:00:00Z
              │   updated_at: 2026-06-09T20:00:00Z
              │   metadata:
              │     type: user
              │   ---
              │   Always indent with tabs in this project.
              │
              ├── db-config.md               ← 另一个事实
              │
              └── frontend-style/            ← 主题子目录
                  ├── naming.md
                  └── patterns.md

  工具
  ====================================================================================================
  remember    保存新事实（模型主动调用，当用户纠正/表达偏好时）
  forget      删除不再需要的事实
  read_memory 读取 model_decision 事实的完整内容（按 slug 名称查询）
              返回: title + description + activation + type + topic + body

  日期标记
  ====================================================================================================
  每条事实在索引中附带最后更新日期（如 "(2026-06-09)"），供模型自行判断时效性：

    - [Prefers tabs](prefers-tabs.md) — User prefers tabs (2026-06-09)

  模型看到日期后自行判断：个人编码风格偏好一年前的也没问题，
  但依赖路径三个月的可能已经变了。

  系统 prompt 已含主动保存指引:
    "when a user corrects you, states a preference, or shares
     non-obvious context about the project, save it with remember.
     Each fact shows its last-updated date in parentheses.
     Judge freshness yourself."
  模型看到索引 → read_memory → 按需获取完整内容
  +=================================================================================================+

  前缀缓存不变性保证
  ====================================================================================================
  +------------------+------------------+------------------+------------------+
  | 时机              | 系统 prompt      | MEMORY.md 磁盘    | 缓存             |
  +------------------+------------------+------------------+------------------+
  | 启动后            | 冻结（旧索引）    | 最新              | 命中 ✅          |
  | 会话中 remember   | 冻结（旧索引）    | 更新 ✅           | 命中 ✅          |
  | 会话中 forget     | 冻结（旧索引）    | 更新 ✅           | 命中 ✅          |
  | 下一轮注入         | 冻结（旧索引）    | —                | 命中 ✅          |
  | 下次启动           | 重新生成（新索引） | 最新              | 需要一次冷启动    |
  +------------------+------------------+------------------+------------------+
