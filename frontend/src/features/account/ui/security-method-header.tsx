import Link from "next/link";
import { ArrowLeft } from "lucide-react";

export function SecurityMethodHeader({
  title,
  description,
  overview = false,
}: {
  title: string;
  description: string;
  overview?: boolean;
}) {
  return (
    <header className="space-y-2">
      <Link
        className="inline-flex min-h-11 items-center gap-2 rounded-md text-sm font-medium text-primary hover:underline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-primary"
        href={
          overview ? "/mi-cuenta/seguridad/metodos" : "/mi-cuenta/seguridad"
        }
      >
        <ArrowLeft className="size-4" aria-hidden="true" />
        {overview ? "Métodos de verificación" : "Seguridad"}
      </Link>
      <h1 className="text-2xl font-bold tracking-tight sm:text-3xl">{title}</h1>
      <p className="text-sm text-muted-foreground">{description}</p>
    </header>
  );
}
