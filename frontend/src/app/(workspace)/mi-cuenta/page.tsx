import { AccountVerificationCard } from "@/features/account/ui/account-verification-card";
import { ProfileCard } from "@/features/account/ui/profile-card";
import { AccountSpacesCard } from "@/features/host-listings/ui/account-spaces-card";
import { ProfilePhotoCard } from "@/features/account/ui/profile-photo-card";
import { SyntheticPayoutCard } from "@/features/account/ui/synthetic-payout-card";

export default function MiCuentaPage() {
  return (
    <div className="space-y-8">
      <AccountVerificationCard />
      <AccountSpacesCard />
      <ProfileCard />
      <ProfilePhotoCard />
      <SyntheticPayoutCard />
    </div>
  );
}
