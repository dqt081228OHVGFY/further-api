import { sanitizeAuthRedirect } from '@/features/auth/lib/auth-redirect';
import { applyAuthBundle } from '@/lib/api';
import type { AuthBundle } from '@/stores/auth-store';
import { postLoginDestination } from './welcome-flow';

export function finishPublicLogin(bundle: AuthBundle, target?: string) {
  applyAuthBundle(bundle);
  location.replace(postLoginDestination(bundle.user, sanitizeAuthRedirect(target, location.origin) || undefined));
}
export const isPublicAuthPath = (path: string) => /^\/(?:setup|oauth|forgot-password|reset|user\/reset|otp)(?:\/|$)/.test(path);
