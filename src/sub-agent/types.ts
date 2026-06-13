export type SubAgentRole =
  | 'diagnostician'
  | 'interviewer'
  | 'researcher'
  | 'reporter'
  | 'jd-analyst'
  | 'resume-optimizer'
  | 'gap-analyzer';

export interface SubAgentConfig {
  id: string;
  role: SubAgentRole;
  systemPrompt: string;
  model?: string;
  maxIterations?: number;
}

export interface SubAgentTask {
  agentId: string;
  input: string;
  parentSessionId: string;
  timeout?: number;
}

export interface SubAgentResult {
  agentId: string;
  role: SubAgentRole;
  output: string;
  tokenUsage: { input: number; output: number };
  duration: number;
  success: boolean;
  error?: string;
}
