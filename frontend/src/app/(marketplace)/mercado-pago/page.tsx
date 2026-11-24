import {
  ArrowRight,
  CheckCircle2,
  CreditCard,
  KeyRound,
  Lock,
  Receipt,
  RefreshCw,
  ShieldCheck,
  Wallet,
} from "lucide-react";
import type { Metadata } from "next";
import Link from "next/link";

export const metadata: Metadata = {
  title: "Total seguridad con Mercado Pago",
  description:
    "Conoce cómo EspaciGo utiliza Mercado Pago: publica gratis, conecta tu cuenta, guardamos tu aprobación para cobrar automáticamente y protege cada arriendo.",
  alternates: { canonical: "/mercado-pago" },
};

export default function MercadoPagoInfoPage() {
  return (
    <article className="mx-auto max-w-6xl px-4 py-12 sm:px-6 sm:py-16 lg:px-8">
      <nav
        aria-label="Breadcrumb"
        className="mb-8 text-sm text-muted-foreground"
      >
        <Link href="/" className="hover:text-foreground">
          Inicio
        </Link>
        <span aria-hidden="true"> / </span>
        <span aria-current="page">Mercado Pago y seguridad</span>
      </nav>

      <header className="rounded-3xl border border-border bg-[var(--brand-surface-subtle)] p-6 sm:p-10">
        <p className="inline-flex items-center gap-2 rounded-full bg-[#009EE3]/10 px-3.5 py-1 text-xs font-semibold uppercase tracking-[0.16em] text-[#0073A8]">
          <ShieldCheck className="size-3.5" aria-hidden="true" />
          Pagos procesados por Mercado Pago
        </p>
        <h1 className="mt-4 max-w-4xl text-3xl font-semibold tracking-tight text-foreground sm:text-5xl">
          Total seguridad con Mercado Pago en cada arriendo
        </h1>
        <p className="mt-5 max-w-3xl text-base leading-relaxed text-[var(--brand-muted-on-subtle)] sm:text-lg">
          Publica gratis en EspaciGo, vincula tu cuenta de Mercado Pago y
          guardamos tu aprobación de forma segura para que puedas cobrar
          automáticamente tus reservas y contratos sin gestionar transferencias
          manuales.
        </p>
        <div className="mt-7 flex flex-wrap gap-3">
          <Link
            href="/acceso?return_to=%2Fanfitrion%2Fmercado-pago"
            className="inline-flex min-h-12 items-center gap-2 rounded-full bg-primary px-6 text-sm font-medium text-primary-foreground transition-colors hover:bg-[var(--brand-primary-hover)]"
          >
            Conectar mi cuenta de Mercado Pago
            <ArrowRight className="size-4" aria-hidden="true" />
          </Link>
          <Link
            href="/acceso?return_to=%2Fanfitrion%2Fespacios%2Fnuevo"
            className="inline-flex min-h-12 items-center rounded-full border border-border bg-background px-6 text-sm font-medium text-foreground hover:bg-secondary"
          >
            Publicar espacio gratis
          </Link>
        </div>
      </header>

      <section className="mt-12 grid gap-6 md:grid-cols-2">
        <div className="rounded-2xl border border-border bg-background p-6 sm:p-8">
          <span className="inline-flex size-11 items-center justify-center rounded-xl bg-[var(--brand-primary-soft)] text-[var(--brand-accent)]">
            <Wallet className="size-5" aria-hidden="true" />
          </span>
          <h2 className="mt-4 text-2xl font-semibold text-foreground">
            Para anfitriones: conecta una vez y cobra automático
          </h2>
          <p className="mt-3 text-sm leading-relaxed text-muted-foreground">
            Publicar tus espacios es gratis. Al conectar tu cuenta de Mercado
            Pago mediante autorización oficial (OAuth), guardamos tu aprobación
            de forma cifrada en el servidor para habilitar el cobro automático
            de tus arriendos aprobados.
          </p>
          <ul className="mt-5 space-y-2.5 text-sm text-foreground">
            <li className="flex items-start gap-2.5">
              <CheckCircle2
                className="mt-0.5 size-4 shrink-0 text-[var(--brand-success)]"
                aria-hidden="true"
              />
              <span>
                <strong>Aprobación guardada:</strong> no necesitas volver a
                autorizar cada publicación o reserva.
              </span>
            </li>
            <li className="flex items-start gap-2.5">
              <RefreshCw
                className="mt-0.5 size-4 shrink-0 text-[var(--brand-success)]"
                aria-hidden="true"
              />
              <span>
                <strong>Cobro y liquidación trazables:</strong> revisa el estado
                de cada pago, comisión y abono en tu panel.
              </span>
            </li>
            <li className="flex items-start gap-2.5">
              <KeyRound
                className="mt-0.5 size-4 shrink-0 text-[var(--brand-success)]"
                aria-hidden="true"
              />
              <span>
                <strong>Control total:</strong> puedes revocar o reconectar tu
                cuenta de Mercado Pago cuando quieras.
              </span>
            </li>
          </ul>
        </div>

        <div className="rounded-2xl border border-border bg-background p-6 sm:p-8">
          <span className="inline-flex size-11 items-center justify-center rounded-xl bg-[#009EE3]/10 text-[#0073A8]">
            <CreditCard className="size-5" aria-hidden="true" />
          </span>
          <h2 className="mt-4 text-2xl font-semibold text-foreground">
            Para arrendatarios: checkout oficial y comprobantes
          </h2>
          <p className="mt-3 text-sm leading-relaxed text-muted-foreground">
            Nunca compartes los datos de tu tarjeta con EspaciGo ni con otros
            usuarios. El pago se realiza directamente en el entorno seguro de
            Mercado Pago y queda vinculado a tu reserva y contrato digital.
          </p>
          <ul className="mt-5 space-y-2.5 text-sm text-foreground">
            <li className="flex items-start gap-2.5">
              <Lock
                className="mt-0.5 size-4 shrink-0 text-[var(--brand-success)]"
                aria-hidden="true"
              />
              <span>
                <strong>Sin exposición de tarjetas:</strong> tus credenciales
                financieras quedan exclusivamente en Mercado Pago.
              </span>
            </li>
            <li className="flex items-start gap-2.5">
              <Receipt
                className="mt-0.5 size-4 shrink-0 text-[var(--brand-success)]"
                aria-hidden="true"
              />
              <span>
                <strong>Confirmación verificada:</strong> validamos el pago con
                Mercado Pago antes de emitir pases y recibos.
              </span>
            </li>
            <li className="flex items-start gap-2.5">
              <ShieldCheck
                className="mt-0.5 size-4 shrink-0 text-[var(--brand-success)]"
                aria-hidden="true"
              />
              <span>
                <strong>Respaldo contractual:</strong> tu pago queda asociado al
                contrato firmado y descargable en PDF.
              </span>
            </li>
          </ul>
        </div>
      </section>

      <section className="mt-12 rounded-3xl border border-border bg-foreground p-6 text-background sm:p-10">
        <h2 className="text-2xl font-semibold tracking-tight sm:text-3xl">
          Seguridad de la plataforma de principio a fin
        </h2>
        <p className="mt-3 max-w-3xl text-sm leading-relaxed text-background/80 sm:text-base">
          EspaciGo opera sobre infraestructura en la nube de AWS con sesiones
          protegidas, llaves de acceso o autenticación en dos pasos (MFA),
          verificación de identidad y almacenamiento privado para documentos
          contractuales y evidencias.
        </p>
        <div className="mt-6 flex flex-wrap gap-3">
          <Link
            href="/acceso?return_to=%2Fmi-cuenta%2Fseguridad"
            className="inline-flex min-h-11 items-center rounded-full bg-primary px-5 text-sm font-medium text-primary-foreground hover:bg-[var(--brand-primary-hover)]"
          >
            Configurar seguridad de mi cuenta
          </Link>
          <Link
            href="/explorar"
            className="inline-flex min-h-11 items-center rounded-full border border-white/25 bg-white/5 px-5 text-sm font-medium text-background hover:bg-white/15"
          >
            Explorar espacios
          </Link>
        </div>
      </section>
    </article>
  );
}
