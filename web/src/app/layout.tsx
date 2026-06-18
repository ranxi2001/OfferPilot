import type { Metadata } from 'next';
import './globals.css';

export const metadata: Metadata = {
  title: 'OfferPilot — AI Interview Diagnosis Agent',
  description: 'AI Agent 面试诊断系统：面试诊断、简历优化、JD 匹配、能力雷达',
};

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="zh-CN">
      <body className="min-h-screen antialiased bg-surface-blue">{children}</body>
    </html>
  );
}
