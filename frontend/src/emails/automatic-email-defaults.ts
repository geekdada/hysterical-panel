import type { AutomaticEmailKind } from "~/api/queries";

// Template content, not UI copy: it is stored and sent as written, so it
// lives here and not in messages/.
type Content = Record<string, unknown>;

const paragraph = (text: string) => ({ type: "paragraph", content: [{ type: "text", text }] });
const divider = { type: "horizontalRule" };

function bilingualDocument(chinese: string[], english: string[]): Content {
  return {
    type: "doc",
    content: [...chinese.map(paragraph), divider, ...english.map(paragraph)],
  };
}

export const automaticEmailDefaults: Record<
  AutomaticEmailKind,
  { subject: string; content: Content }
> = {
  low_allowance: {
    subject: "流量即将用完 / Your traffic is running low",
    content: bilingualDocument(
      [
        "你的当前订阅流量即将用完。",
        "剩余 {{remaining}}（{{remaining_percent}}%），总额度 {{allowance}}。",
        "当前周期结束时间：{{window_ends_at}}。",
      ],
      [
        "Your traffic allowance is running low.",
        "Remaining: {{remaining}} ({{remaining_percent}}%) of {{allowance}}.",
        "The current period ends at {{window_ends_at}}.",
      ]
    ),
  },
  allowance_exhausted: {
    subject: "流量已用完 / Your traffic is used up",
    content: bilingualDocument(
      [
        "你的当前订阅流量已用完，新连接将被拒绝。",
        "总额度 {{allowance}}。额度将在 {{window_ends_at}} 重置。",
      ],
      [
        "Your traffic allowance is used up and new connections are refused.",
        "Allowance: {{allowance}}. It resets at {{window_ends_at}}.",
      ]
    ),
  },
  allowance_restored: {
    subject: "流量已恢复 / Your traffic is available again",
    content: bilingualDocument(
      [
        "你的订阅流量已恢复，可以继续使用。",
        "剩余 {{remaining}}。当前周期结束时间：{{window_ends_at}}。",
      ],
      [
        "Your traffic allowance is available again.",
        "Remaining: {{remaining}}. The current period ends at {{window_ends_at}}.",
      ]
    ),
  },
};
