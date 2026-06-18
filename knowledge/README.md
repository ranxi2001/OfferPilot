# 知识库 (Knowledge Base)

## 目录结构

```
knowledge/
├── 01-architecture-design/    # 架构选型（34题）
├── 02-tool-management/        # 工具管理（25题）
├── 03-fault-tolerance/        # 容错与鲁棒性（23题）
├── 04-memory-context/         # 记忆与上下文（46题）
├── 05-eval-and-vision/        # 评估与全局观（28题）
├── 06-multi-agent-collab/     # 多智能体协作（20题）
├── 07-engineering-pitfalls/   # 工程化踩坑（47题）
├── 08-prompt-engineering/     # Prompt 工程（16题）
├── 09-rag-retrieval/          # RAG 与检索（54题）
├── 10-training-and-data/      # 训练与模型（45题）
├── 11-ai-code-testing/        # AI 代码测试（7题）
├── 12-business-ai-engineering/# 业务 AI 工程（7题）
├── 13-project-deep-dive/      # 简历项目拷打（20题）
├── 14-company-preferences/    # 公司偏好
├── 15-agent-concepts/         # Agent 概念（12题）
└── coaching-methodology/      # 辅导方法论（非题库）
    ├── resume-rewriting.md    # 方向感知简历包装策略（4方向黄金句式）
    ├── interview-coaching.md  # 面试辅导：L1-L5 层次模型与差距诊断
    ├── job-hunting-strategy.md# 求职策略：批次投递、面试复盘、3故事法
    ├── salary-negotiation.md  # 薪资谈判：competing offer 杠杆、总包拆解
    ├── behavioral-rubric.md   # 行为面 STAR 五维评分（1-5）+ 系统设计六轴
    ├── ats-scoring-rubric.md  # ATS 7 维度加权评分（A-F 等级）
    └── anti-ai-detection.md   # 反 AI 检测：5种失败模式 + buzzword 黑名单
```

## 文件格式

每个 `.md` 文件代表一道面试题，格式如下：

```markdown
# [维度] - [题目标题]

## Q：[面试问题]

> 来源：[出处]

**新手答**："..."

**高手答**：

[详细的专家级回答，包含工程细节和实践经验]

## 考察点

- [考察维度1]
- [考察维度2]

## 追问

- [可能的追问1]
- [可能的追问2]
```

## 导入知识库

```bash
# 将 Markdown 文件解析并写入 SQLite
npm run build-kb -- --dir knowledge --db data/agent.db
```

## 外部知识源导入

支持从以下格式批量导入：
1. 本目录的 Markdown 文件（推荐格式）
2. JSON 数组文件（需符合 KnowledgeEntry 接口）

### JSON 格式示例

```json
[
  {
    "id": "unique-id",
    "title": "题目标题",
    "dimension": "architecture",
    "content": "完整内容",
    "question": "面试问题",
    "expertAnswer": "专家答案",
    "noviceAnswer": "新手答案",
    "tags": ["tag1", "tag2"]
  }
]
```

## 贡献知识

1. 在对应维度目录下创建 `.md` 文件
2. 按上述格式填写内容
3. 运行 `npm run build-kb` 重新索引
