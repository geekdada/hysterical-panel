import { twMerge, type ClassNameValue } from "tailwind-merge";

// Merge Tailwind class strings, resolving conflicts so later classes win.
// Shared UI primitives use this to fold an externally-passed `className` into
// their base styles: a caller's `text-(--foreground)` overrides the base
// `text-(--muted)` instead of both being emitted with source-order ambiguity.
export function cn(...inputs: ClassNameValue[]) {
  return twMerge(inputs);
}
