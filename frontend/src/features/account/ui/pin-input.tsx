import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";

export function PinInput({
  id,
  label,
  value,
  onChange,
  disabled,
}: {
  id: string;
  label: string;
  value: string;
  onChange: (value: string) => void;
  disabled: boolean;
}) {
  return (
    <div className="space-y-2">
      <Label htmlFor={id}>
        {label} <span className="text-muted-foreground">(6 dígitos)</span>
      </Label>
      <div className="relative grid w-full max-w-[19rem] grid-cols-6 gap-1.5 rounded-lg focus-within:ring-2 focus-within:ring-ring focus-within:ring-offset-2 sm:gap-2">
        {Array.from({ length: 6 }, (_, index) => (
          <span
            key={index}
            aria-hidden="true"
            className={`flex h-12 min-w-0 items-center justify-center rounded-lg border text-xl font-semibold ${value[index] ? "border-primary bg-primary/10 text-primary" : "border-input bg-background"}`}
          >
            {value[index] ? "•" : ""}
          </span>
        ))}
        <Input
          id={id}
          type="password"
          inputMode="numeric"
          autoComplete="one-time-code"
          pattern="[0-9]{6}"
          maxLength={6}
          value={value}
          onChange={(event) =>
            onChange(event.target.value.replace(/\D/g, "").slice(0, 6))
          }
          disabled={disabled}
          className="absolute inset-0 z-10 h-full w-full cursor-text opacity-0"
          aria-describedby={`${id}-hint`}
          aria-label={label}
          required
        />
      </div>
      <p id={`${id}-hint`} className="text-xs text-muted-foreground">
        Escribe o pega los seis dígitos.
      </p>
    </div>
  );
}
