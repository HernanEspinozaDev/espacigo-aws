import "client-only";

import { apiClient } from "@/lib/api/client";
import { ApiProtocolError, normalizeApiError } from "@/lib/api/errors";
import { serializeExactJson } from "@/lib/api/exact-json";
import type { AccountPort } from "../application/account.port";
import type { Profile } from "../domain/profile";
import type { ProfilePhotoMetadata } from "../domain/photo";
import {
  normalizeSyntheticPayoutAccount,
  type SyntheticPayoutAccount,
} from "../domain/payout-account";
import type {
  OwnIdentityExport,
  PrivacyPortabilityBlobResult,
  PrivacyPurposePreference,
  PrivacyTreatmentBlock,
  RightsRequest,
  RightsRequestKind,
  UpdatePrivacyPurposePreferenceInput,
} from "../domain/privacy";

export class OpenApiAccountAdapter implements AccountPort {
  async getProfile(signal?: AbortSignal): Promise<Profile | null> {
    const result = await apiClient.GET("/profile", {
      signal,
    });
    if (result.response.status === 404) return null;
    if (!result.response.ok) {
      throw normalizeApiError(result.response, result.error);
    }
    if (!result.data) throw new ApiProtocolError();
    return {
      displayName: result.data.display_name,
      phone: result.data.phone ?? null,
      updatedAt: result.data.updated_at,
    };
  }

  async updateProfile(
    payload: { displayName: string; phone?: string },
    signal?: AbortSignal,
  ): Promise<Profile> {
    const result = await apiClient.PUT("/profile", {
      body: {
        display_name: payload.displayName,
        ...(payload.phone !== undefined ? { phone: payload.phone } : {}),
      },
      bodySerializer: serializeExactJson,
      signal,
    });
    if (!result.response.ok) {
      throw normalizeApiError(result.response, result.error);
    }
    if (!result.data) throw new ApiProtocolError();
    return {
      displayName: result.data.display_name,
      phone: result.data.phone ?? null,
      updatedAt: result.data.updated_at,
    };
  }

  async getProfilePhoto(
    signal?: AbortSignal,
  ): Promise<ProfilePhotoMetadata | null> {
    const result = await apiClient.GET("/profile/photo", {
      signal,
    });
    if (result.response.status === 404) return null;
    if (!result.response.ok) {
      throw normalizeApiError(result.response, result.error);
    }
    if (!result.data) throw new ApiProtocolError();
    const data = result.data;
    if ("fixture_code" in data) {
      return {
        id: data.id,
        fixtureCode: "synthetic-png-v1",
        mimeType: "image/png",
        sha256: data.sha256,
        sizeBytes: data.size_bytes,
        createdAt: data.created_at,
        storageKind: "synthetic_fixed",
      };
    }
    const variant = data.variants?.[0];
    if (!variant) throw new ApiProtocolError();
    return {
      id: data.id,
      mimeType: variant.mime_type,
      sha256: variant.sha256,
      sizeBytes: variant.size_bytes,
      width: variant.width,
      height: variant.height,
      createdAt: data.created_at,
      storageKind: "uploaded_avatar",
    };
  }

  async generateSyntheticPhoto(
    idempotencyKey: string,
    signal?: AbortSignal,
  ): Promise<void> {
    const result = await apiClient.PUT("/profile/photo", {
      params: {
        header: {
          "Idempotency-Key": idempotencyKey,
        },
      },
      signal,
    });
    if (!result.response.ok) {
      throw normalizeApiError(result.response, result.error);
    }
  }

  async uploadProfilePhoto(
    idempotencyKey: string,
    imageFile: Blob | File,
    signal?: AbortSignal,
  ): Promise<void> {
    const formData = new FormData();
    formData.append("image", imageFile);
    const result = await apiClient.PUT("/profile/photo", {
      params: {
        header: {
          "Idempotency-Key": idempotencyKey,
        },
      },
      body: formData as unknown as { image: string },
      signal,
    });
    if (!result.response.ok) {
      throw normalizeApiError(result.response, result.error);
    }
  }

  async removeProfilePhoto(
    idempotencyKey: string,
    signal?: AbortSignal,
  ): Promise<void> {
    const result = await apiClient.DELETE("/profile/photo", {
      params: {
        header: {
          "Idempotency-Key": idempotencyKey,
        },
      },
      signal,
    });
    if (result.response.status === 404) return;
    if (!result.response.ok) {
      throw normalizeApiError(result.response, result.error);
    }
  }

  async getProfilePhotoContent(signal?: AbortSignal): Promise<Blob | null> {
    const result = await apiClient.GET("/profile/photo/content", {
      parseAs: "blob",
      signal,
    });
    if (result.response.status === 404) return null;
    if (!result.response.ok) {
      throw normalizeApiError(result.response, result.error);
    }
    return (result.data as Blob) ?? (await result.response.blob());
  }

  async getPayoutAccount(
    signal?: AbortSignal,
  ): Promise<SyntheticPayoutAccount | null> {
    const result = await apiClient.GET("/payout-account", {
      signal,
    });
    if (result.response.status === 404) return null;
    if (!result.response.ok) {
      throw normalizeApiError(result.response, result.error);
    }
    return normalizeSyntheticPayoutAccount(result.data);
  }

  async createOrReplacePayoutAccount(
    idempotencyKey: string,
    signal?: AbortSignal,
  ): Promise<SyntheticPayoutAccount> {
    const result = await apiClient.PUT("/payout-account", {
      params: {
        header: {
          "Idempotency-Key": idempotencyKey,
        },
      },
      signal,
    });
    if (!result.response.ok) {
      throw normalizeApiError(result.response, result.error);
    }
    const parsed = normalizeSyntheticPayoutAccount(result.data);
    if (!parsed) {
      return {
        referenceId: `payout-${idempotencyKey.slice(0, 8)}`,
        status: "activa_fake",
        notice: "Referencia local ficticia; no contiene datos bancarios.",
      };
    }
    return parsed;
  }

  async revokePayoutAccount(
    idempotencyKey: string,
    signal?: AbortSignal,
  ): Promise<void> {
    const result = await apiClient.DELETE("/payout-account", {
      params: {
        header: {
          "Idempotency-Key": idempotencyKey,
        },
      },
      signal,
    });
    if (result.response.status === 404) return;
    if (!result.response.ok) {
      throw normalizeApiError(result.response, result.error);
    }
  }

  async listRightsRequests(signal?: AbortSignal): Promise<RightsRequest[]> {
    const result = await apiClient.GET("/rights-requests", {
      signal,
    });
    if (!result.response.ok) {
      throw normalizeApiError(result.response, result.error);
    }
    return result.data?.items ?? [];
  }

  async requestDataRight(
    kind: RightsRequestKind,
    detail: string,
    signal?: AbortSignal,
  ): Promise<RightsRequest> {
    const result = await apiClient.POST("/rights-requests", {
      body: { type: kind, ...(detail ? { detail } : {}) } as never,
      bodySerializer: serializeExactJson,
      signal,
    });
    if (!result.response.ok) {
      throw normalizeApiError(result.response, result.error);
    }
    if (!result.data) throw new ApiProtocolError();
    return result.data;
  }

  async exportIdentityData(
    signal?: AbortSignal,
  ): Promise<OwnIdentityExport | null> {
    const result = await apiClient.GET("/privacy/export", {
      signal,
    });
    if (result.response.status === 404) return null;
    if (!result.response.ok) {
      throw normalizeApiError(result.response, result.error);
    }
    return result.data ?? null;
  }

  async exportIdentityBlob(signal?: AbortSignal): Promise<Blob | null> {
    const result = await apiClient.GET("/privacy/export", {
      parseAs: "blob",
      signal,
    });
    if (result.response.status === 404) return null;
    if (!result.response.ok) {
      throw normalizeApiError(result.response, result.error);
    }
    return (result.data as Blob) ?? (await result.response.blob());
  }

  async downloadIdentityArchive(signal?: AbortSignal): Promise<Blob | null> {
    const result = await apiClient.GET("/privacy/export/archive", {
      parseAs: "blob",
      signal,
    });
    if (result.response.status === 404) return null;
    if (!result.response.ok) {
      throw normalizeApiError(result.response, result.error);
    }
    return (result.data as Blob) ?? (await result.response.blob());
  }

  async listPrivacyPreferences(
    signal?: AbortSignal,
  ): Promise<PrivacyPurposePreference[]> {
    const result = await apiClient.GET("/privacy/preferences", {
      signal,
    });
    if (!result.response.ok) {
      throw normalizeApiError(result.response, result.error);
    }
    return result.data?.items ?? [];
  }

  async updatePrivacyPreference(
    input: UpdatePrivacyPurposePreferenceInput,
    signal?: AbortSignal,
  ): Promise<PrivacyPurposePreference> {
    const trimmedReason = input.reason?.trim();
    const result = await apiClient.PUT("/privacy/preferences", {
      body: {
        purpose: input.purpose,
        state: input.state,
        ...(trimmedReason ? { reason: trimmedReason } : {}),
      },
      bodySerializer: serializeExactJson,
      signal,
    });
    if (!result.response.ok) {
      throw normalizeApiError(result.response, result.error);
    }
    if (!result.data?.data) throw new ApiProtocolError();
    return result.data.data;
  }

  async listPrivacyRestrictions(
    signal?: AbortSignal,
  ): Promise<PrivacyTreatmentBlock[]> {
    const result = await apiClient.GET("/privacy/restrictions", {
      signal,
    });
    if (!result.response.ok) {
      throw normalizeApiError(result.response, result.error);
    }
    return result.data?.items ?? [];
  }

  async exportPortabilityBlob(
    signal?: AbortSignal,
  ): Promise<PrivacyPortabilityBlobResult | null> {
    const result = await apiClient.GET("/privacy/portability", {
      parseAs: "blob",
      signal,
    });
    if (result.response.status === 404) return null;
    if (!result.response.ok) {
      throw normalizeApiError(result.response, result.error);
    }
    const blob = (result.data as Blob) ?? (await result.response.blob());
    const rawSha = result.response.headers.get("X-Content-SHA256");
    return {
      blob,
      sha256: rawSha && rawSha.trim() ? rawSha.trim() : null,
    };
  }
}
