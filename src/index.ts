import './env.js';
import { Command } from 'commander';
import { createInterface } from 'node:readline';
import { resolve } from 'node:path';
import chalk from 'chalk';
import { createApp } from './app.js';
import { openDatabase, initSchema } from './db/index.js';
import { parseKnowledgeDir, OpenAIEmbeddingProvider } from './knowledge/index.js';
import { KnowledgeSearch } from './knowledge/search.js';

const program = new Command();

program
  .name('interview-agent')
  .description('面试诊断 Agent —— AI Agent 面试辅导系统')
  .version('0.1.0');

program
  .command('start')
  .description('启动交互式诊断会话')
  .option('-m, --model <model>', '指定 LLM 模型')
  .action(async (opts) => {
    const app = createApp({
      model: opts.model,
      onTextDelta: (text) => process.stdout.write(text),
      onToolCall: (name) => {
        process.stdout.write(chalk.dim(`\n[调用工具: ${name}]\n`));
      },
    });

    const session = app.sessionManager.create();
    console.log(chalk.green(`\n面试诊断 Agent 已启动 (session: ${session.id.slice(0, 8)})`));
    console.log(chalk.dim('输入面试题开始诊断，输入 /help 查看命令\n'));

    const rl = createInterface({
      input: process.stdin,
      output: process.stdout,
    });

    setupGracefulShutdown(() => rl.close());

    const prompt = () => {
      rl.question(chalk.blue('> '), async (input) => {
        const trimmed = input.trim();
        if (!trimmed) return prompt();

        // Handle slash commands
        if (app.commandParser.isCommand(trimmed)) {
          const result = await app.commandParser.execute(trimmed, {
            sessionId: session.id,
            app,
          });
          console.log(chalk.dim(result.output));

          if (!result.shouldContinue) {
            rl.close();
            return;
          }

          if (result.metadata?.action === 'reset') {
            const newSession = app.sessionManager.create();
            Object.assign(session, newSession);
            console.log(chalk.green(`新会话: ${session.id.slice(0, 8)}\n`));
          }

          return prompt();
        }

        try {
          console.log();
          await app.agent.run(session.id, trimmed);
          console.log('\n');
        } catch (err) {
          console.error(chalk.red(`\n错误: ${(err as Error).message}\n`));
        }

        prompt();
      });
    };

    prompt();
  });

program
  .command('diagnose')
  .description('单次诊断模式：输入问题和回答，直接输出诊断结果')
  .requiredOption('-q, --question <question>', '面试题目')
  .requiredOption('-a, --answer <answer>', '你的回答')
  .option('-m, --model <model>', '指定 LLM 模型')
  .action(async (opts) => {
    const app = createApp({
      model: opts.model,
      onTextDelta: (text) => process.stdout.write(text),
    });

    const session = app.sessionManager.create();
    const userInput = `请诊断我对这个面试题的回答：\n\n题目：${opts.question}\n\n我的回答：${opts.answer}`;

    try {
      await app.agent.run(session.id, userInput);
      console.log('\n');
    } catch (err) {
      console.error(chalk.red(`错误: ${(err as Error).message}`));
      process.exit(1);
    }
  });

program
  .command('build-kb')
  .description('解析知识库 Markdown 文件并写入 SQLite')
  .option('-d, --dir <path>', '知识库目录', 'knowledge')
  .option('--db <path>', '数据库路径', 'data/agent.db')
  .action(async (opts) => {
    const knowledgeDir = resolve(opts.dir);
    const dbPath = resolve(opts.db);

    console.log(chalk.dim(`解析知识库: ${knowledgeDir}`));
    const entries = parseKnowledgeDir(knowledgeDir);
    console.log(chalk.green(`解析完成: ${entries.length} 条知识条目`));

    const db = openDatabase(dbPath);
    initSchema(db);

    const search = new KnowledgeSearch(db);
    search.bulkInsert(entries);

    console.log(chalk.green(`写入数据库: ${search.count()} 条 (${dbPath})`));
    db.close();
  });

program
  .command('embed')
  .description('为知识库生成 embedding 向量索引（需要 OPENAI_API_KEY）')
  .option('--db <path>', '数据库路径', 'data/agent.db')
  .option('--model <model>', 'Embedding 模型', 'text-embedding-3-small')
  .action(async (opts) => {
    if (!process.env.OPENAI_API_KEY) {
      console.error(chalk.red('错误: 需要设置 OPENAI_API_KEY 环境变量'));
      process.exit(1);
    }

    const dbPath = resolve(opts.db);
    const db = openDatabase(dbPath);
    initSchema(db);

    const embeddingProvider = new OpenAIEmbeddingProvider({ model: opts.model });
    const search = new KnowledgeSearch(db, embeddingProvider);

    const total = search.count();
    console.log(chalk.dim(`知识库共 ${total} 条，开始生成 embedding...`));

    const indexed = await search.indexEmbeddings();
    console.log(chalk.green(`完成: 新生成 ${indexed} 条 embedding 向量 (模型: ${opts.model})`));
    db.close();
  });

function setupGracefulShutdown(cleanup: () => void) {
  let shuttingDown = false;
  const handler = () => {
    if (shuttingDown) return;
    shuttingDown = true;
    console.log(chalk.dim('\n正在关闭...'));
    cleanup();
    process.exit(0);
  };
  process.on('SIGTERM', handler);
  process.on('SIGINT', handler);
}

program.parse();
