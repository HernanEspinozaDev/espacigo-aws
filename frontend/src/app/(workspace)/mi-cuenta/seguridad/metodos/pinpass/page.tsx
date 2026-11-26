import { PinpassAccountSection } from "@/features/account/ui/pinpass-card";
import { SecurityMethodHeader } from "@/features/account/ui/security-method-header";

export default function PinpassPage() {
  return (
    <div className="space-y-6">
      <SecurityMethodHeader
        overview
        title="Pinpass"
        description="Administra tu clave de seis dígitos para desbloquear opciones y consultar tus datos privados."
      />
      <PinpassAccountSection manage />
    </div>
  );
}
