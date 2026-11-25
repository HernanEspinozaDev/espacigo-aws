import { SecurityCard } from "@/features/account/ui/security-card";
import { SecurityMethodHeader } from "@/features/account/ui/security-method-header";

export default function ContrasenaMetodoPage() {
  return (
    <div className="space-y-6">
      <SecurityMethodHeader
        title="Contraseña"
        description="Ten a mano tu contraseña actual para cambiarla."
        overview
      />
      <SecurityCard />
    </div>
  );
}
