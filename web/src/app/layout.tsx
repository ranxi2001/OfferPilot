import type { Metadata } from 'next';
import './globals.css';

export const metadata: Metadata = {
  title: 'OfferPilot — AI Interview Diagnosis Agent',
  description: 'AI Agent 面试诊断系统：面试诊断、简历优化、JD 匹配、能力雷达',
  icons: {
    icon: [
      { url: '/brand/offerpilot-favicon-32.png', sizes: '32x32', type: 'image/png' },
      { url: '/brand/offerpilot-icon-192.png', sizes: '192x192', type: 'image/png' },
    ],
    apple: [{ url: '/brand/offerpilot-icon-192.png', sizes: '192x192', type: 'image/png' }],
  },
};

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="zh-CN">
      <body className="min-h-screen antialiased bg-surface-blue">{children}</body>
    </html>
  );
}
