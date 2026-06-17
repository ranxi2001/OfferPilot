import type { ToolSchema } from '../query-engine/types.js';

export type RiskLevel = 'low' | 'medium' | 'high' | 'critical';

export interface ToolDefinition {
  schema: ToolSchema;
  riskLevel: RiskLevel;
  execute: (input: Record<string, unknown>, ctx: ToolContext) => Promise<ToolResult>;
}

export interface ToolContext {
  sessionId: string;
  userId?: string;
  abortSignal?: AbortSignal;
}

export interface ToolResult {
  success: boolean;
  output: string;
  isError?: boolean;
  metadata?: Record<string, unknown>;
}
