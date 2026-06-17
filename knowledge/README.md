# 知识库 (Knowledge Base)

## 目录结构

```
knowledge/
├── 01-architecture/   # 架构设计类面试题
├── 02-engineering/    # Harness 工程类
├── 03-model/          # 模型能力类
├── 04-rag/            # RAG 知识增强类
├── 05-multi-agent/    # 多 Agent 协作类
├── 06-evaluation/     # 评测质量类
└── 07-full-stack/     # 全栈工程类
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
