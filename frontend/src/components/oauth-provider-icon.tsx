import type { ComponentType, SVGProps } from "react";
import { LogoYandex } from "@gravity-ui/icons";
import {
  siApple,
  siBitbucket,
  siBox,
  siDiscord,
  siFacebook,
  siGitea,
  siGitee,
  siGithub,
  siGitlab,
  siGoogle,
  siInstagram,
  siKakaotalk,
  siLinear,
  siLivechat,
  siNotion,
  siPatreon,
  siSpotify,
  siStrava,
  siTrakt,
  siTwitch,
  siVk,
  siWakatime,
  siX,
  type SimpleIcon,
} from "simple-icons";
import { cn } from "~/lib/cn";

type IconProps = SVGProps<SVGSVGElement>;

// Keys are PocketBase provider names. Generic OIDC slots and providers
// without a known mark render no icon.
const SIMPLE_ICONS: Record<string, SimpleIcon> = {
  apple: siApple,
  bitbucket: siBitbucket,
  box: siBox,
  discord: siDiscord,
  facebook: siFacebook,
  gitea: siGitea,
  gitee: siGitee,
  github: siGithub,
  gitlab: siGitlab,
  google: siGoogle,
  instagram2: siInstagram,
  kakao: siKakaotalk,
  linear: siLinear,
  livechat: siLivechat,
  notion: siNotion,
  patreon: siPatreon,
  spotify: siSpotify,
  strava: siStrava,
  trakt: siTrakt,
  twitch: siTwitch,
  twitter: siX,
  vk: siVk,
  wakatime: siWakatime,
};

// simple-icons dropped the Microsoft marks, so this file draws the four squares.
function MicrosoftLogo(props: IconProps) {
  return (
    <svg viewBox="0 0 24 24" fill="currentColor" {...props}>
      <rect x="1" y="1" width="10.5" height="10.5" />
      <rect x="12.5" y="1" width="10.5" height="10.5" />
      <rect x="1" y="12.5" width="10.5" height="10.5" />
      <rect x="12.5" y="12.5" width="10.5" height="10.5" />
    </svg>
  );
}

const CUSTOM_ICONS: Record<string, ComponentType<IconProps>> = {
  microsoft: MicrosoftLogo,
  yandex: LogoYandex,
};

/** Monochrome mark for a recognised OAuth2 provider, or nothing. */
export function OAuthProviderIcon({
  provider,
  className,
}: {
  provider: string;
  className?: string;
}) {
  const iconClassName = cn("size-4 shrink-0", className);
  const Custom = CUSTOM_ICONS[provider];
  if (Custom) return <Custom aria-hidden className={iconClassName} />;
  const icon = SIMPLE_ICONS[provider];
  if (!icon) return null;
  return (
    <svg viewBox="0 0 24 24" fill="currentColor" aria-hidden className={iconClassName}>
      <path d={icon.path} />
    </svg>
  );
}
