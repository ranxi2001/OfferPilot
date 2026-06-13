import { describe, it, expect } from 'vitest';
import { createToolRegistry } from '../../src/tools/index.js';

describe('ToolRegistry', () => {
  const registry = createToolRegistry();

  it('should register all builtin tools', () => {
    const schemas = registry.listSchemas();
    expect(schemas.length).toBe(13);
  });

  it('should find tools by name', () => {
    expect(registry.has('search_knowledge')).toBe(true);
    expect(registry.has('diagnose_answer')).toBe(true);
    expect(registry.has('nonexistent')).toBe(false);
  });

  it('should execute a tool', async () => {
    const result = await registry.execute(
      'list_dimensions',
      {},
      { sessionId: 'test-session' },
    );
    expect(result.success).toBe(true);
    const data = JSON.parse(result.output);
    expect(data.dimensions).toHaveLength(7);
  });

  it('should return error for unknown tool', async () => {
    const result = await registry.execute(
      'unknown_tool',
      {},
      { sessionId: 'test-session' },
    );
    expect(result.success).toBe(false);
    expect(result.output).toContain('not found');
  });
});
