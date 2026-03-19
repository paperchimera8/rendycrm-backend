import { createHmac, timingSafeEqual } from "node:crypto";

export type AuthClaims = {
  email: string;
  exp: number;
};

function base64UrlEncode(value: string): string {
  return Buffer.from(value, "utf8").toString("base64url");
}

function base64UrlDecode(value: string): string {
  return Buffer.from(value, "base64url").toString("utf8");
}

function signToken(secret: string, payload: string): string {
  return createHmac("sha256", secret).update(payload).digest("base64url");
}

export function issueToken(secret: string, email: string, ttlMs: number): { token: string; expiresAt: Date } {
  const expiresAt = new Date(Date.now() + ttlMs);
  const claims: AuthClaims = {
    email,
    exp: Math.floor(expiresAt.getTime() / 1000),
  };
  const body = base64UrlEncode(JSON.stringify(claims));
  const signature = signToken(secret, body);
  return {
    token: `${body}.${signature}`,
    expiresAt,
  };
}

export function parseToken(secret: string, token: string): AuthClaims {
  const [body, signature, ...rest] = token.split(".");
  if (!body || !signature || rest.length > 0) {
    throw new Error("bad token format");
  }

  const expectedSignature = signToken(secret, body);
  if (
    expectedSignature.length !== signature.length ||
    !timingSafeEqual(Buffer.from(expectedSignature), Buffer.from(signature))
  ) {
    throw new Error("bad signature");
  }

  const claims = JSON.parse(base64UrlDecode(body)) as Partial<AuthClaims>;
  if (typeof claims.email !== "string" || claims.email.trim() === "") {
    throw new Error("token email is missing");
  }
  if (typeof claims.exp !== "number" || !Number.isFinite(claims.exp)) {
    throw new Error("token expiration is invalid");
  }
  if (claims.exp <= Math.floor(Date.now() / 1000)) {
    throw new Error("token expired");
  }

  return {
    email: claims.email,
    exp: claims.exp,
  };
}
