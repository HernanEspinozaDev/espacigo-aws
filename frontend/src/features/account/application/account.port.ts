import type { Profile } from "../domain/profile";
import type { ProfilePhotoMetadata } from "../domain/photo";
import type { SyntheticPayoutAccount } from "../domain/payout-account";
import type {
  OwnIdentityExport,
  PrivacyPortabilityBlobResult,
  PrivacyPurposePreference,
  PrivacyTreatmentBlock,
  RightsRequest,
  RightsRequestKind,
  UpdatePrivacyPurposePreferenceInput,
} from "../domain/privacy";

export interface AccountPort {
  getProfile(signal?: AbortSignal): Promise<Profile | null>;
  updateProfile(
    payload: { displayName: string; phone?: string },
    signal?: AbortSignal,
  ): Promise<Profile>;

  getProfilePhoto(signal?: AbortSignal): Promise<ProfilePhotoMetadata | null>;
  generateSyntheticPhoto(
    idempotencyKey: string,
    signal?: AbortSignal,
  ): Promise<void>;
  uploadProfilePhoto(
    idempotencyKey: string,
    imageFile: Blob | File,
    signal?: AbortSignal,
  ): Promise<void>;
  removeProfilePhoto(
    idempotencyKey: string,
    signal?: AbortSignal,
  ): Promise<void>;
  getProfilePhotoContent(signal?: AbortSignal): Promise<Blob | null>;

  getPayoutAccount(
    signal?: AbortSignal,
  ): Promise<SyntheticPayoutAccount | null>;
  createOrReplacePayoutAccount(
    idempotencyKey: string,
    signal?: AbortSignal,
  ): Promise<SyntheticPayoutAccount>;
  revokePayoutAccount(
    idempotencyKey: string,
    signal?: AbortSignal,
  ): Promise<void>;

  listRightsRequests(signal?: AbortSignal): Promise<RightsRequest[]>;
  requestDataRight(
    kind: RightsRequestKind,
    detail: string,
    signal?: AbortSignal,
  ): Promise<RightsRequest>;

  exportIdentityData(signal?: AbortSignal): Promise<OwnIdentityExport | null>;
  exportIdentityBlob(signal?: AbortSignal): Promise<Blob | null>;
  downloadIdentityArchive(signal?: AbortSignal): Promise<Blob | null>;

  listPrivacyPreferences(
    signal?: AbortSignal,
  ): Promise<PrivacyPurposePreference[]>;
  updatePrivacyPreference(
    input: UpdatePrivacyPurposePreferenceInput,
    signal?: AbortSignal,
  ): Promise<PrivacyPurposePreference>;
  listPrivacyRestrictions(
    signal?: AbortSignal,
  ): Promise<PrivacyTreatmentBlock[]>;
  exportPortabilityBlob(
    signal?: AbortSignal,
  ): Promise<PrivacyPortabilityBlobResult | null>;
}
