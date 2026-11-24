import Image from "next/image";
import Link from "next/link";

export function Footer() {
  const year = new Date().getFullYear();

  return (
    <footer className="border-t border-border bg-[var(--brand-surface-subtle)]">
      <div className="mx-auto max-w-7xl px-4 sm:px-6 lg:px-8">
        {/* Columnas principales */}
        <div className="grid grid-cols-1 gap-10 py-12 sm:grid-cols-2 lg:grid-cols-[1.4fr_1fr_1fr_1fr]">
          <div>
            <Link
              href="/"
              className="inline-flex items-center gap-2 text-xl font-semibold tracking-tight text-foreground"
            >
              <Image
                src="/espacigo-mark.svg"
                width={28}
                height={28}
                alt=""
              />
              <span>EspaciGo</span>
            </Link>
            <p className="mt-4 max-w-xs text-sm leading-relaxed text-[var(--brand-muted-on-subtle)]">
              Conectamos personas con espacios. Arriendos residenciales y
              espacios flexibles con contratos digitales, mapa interactivo y
              pagos en línea.
            </p>
          </div>

          <div>
            <h2 className="text-sm font-semibold text-foreground">Explora</h2>
            <ul className="mt-4 space-y-2.5">
              <li>
                <Link
                  href="/explorar"
                  className="text-sm text-[var(--brand-muted-on-subtle)] hover:text-primary"
                >
                  Catálogo público
                </Link>
              </li>
              <li>
                <Link
                  href="/explorar/mapa"
                  className="text-sm text-[var(--brand-muted-on-subtle)] hover:text-primary"
                >
                  Mapa de espacios
                </Link>
              </li>
              <li>
                <Link
                  href="/categorias"
                  className="text-sm text-[var(--brand-muted-on-subtle)] hover:text-primary"
                >
                  Categorías y guías
                </Link>
              </li>
              <li>
                <Link
                  href="/arriendos"
                  className="text-sm text-[var(--brand-muted-on-subtle)] hover:text-primary"
                >
                  Arriendos residenciales
                </Link>
              </li>
            </ul>
          </div>

          <div>
            <h2 className="text-sm font-semibold text-foreground">
              Arrendadores y Pagos
            </h2>
            <ul className="mt-4 space-y-2.5">
              <li>
                <Link
                  href="/como-funciona?rol=arrendador"
                  className="text-sm text-[var(--brand-muted-on-subtle)] hover:text-primary"
                >
                  Cómo funciona · Arrendadores
                </Link>
              </li>
              <li>
                <Link
                  href="/acceso?return_to=%2Fanfitrion%2Fespacios%2Fnuevo"
                  className="text-sm text-[var(--brand-muted-on-subtle)] hover:text-primary"
                >
                  Publica gratis
                </Link>
              </li>
              <li>
                <Link
                  href="/mercado-pago"
                  className="text-sm text-[var(--brand-muted-on-subtle)] hover:text-primary"
                >
                  Seguridad con Mercado Pago
                </Link>
              </li>
              <li>
                <Link
                  href="/acceso"
                  className="text-sm text-[var(--brand-muted-on-subtle)] hover:text-primary"
                >
                  Iniciar sesión
                </Link>
              </li>
            </ul>
          </div>

          <div>
            <h2 className="text-sm font-semibold text-foreground">
              Información legal
            </h2>
            <ul className="mt-4 space-y-2.5">
              <li>
                <Link
                  href="/terminos-y-condiciones"
                  className="text-sm text-[var(--brand-muted-on-subtle)] hover:text-primary"
                >
                  Términos y condiciones
                </Link>
              </li>
              <li>
                <Link
                  href="/politica-de-privacidad"
                  className="text-sm text-[var(--brand-muted-on-subtle)] hover:text-primary"
                >
                  Política de privacidad
                </Link>
              </li>
            </ul>
          </div>
        </div>

        {/* Infraestructura y procesamiento de pagos con logotipos oficiales */}
        <div className="flex flex-col gap-6 border-t border-border py-6 sm:flex-row sm:items-end sm:justify-between">
          <div className="flex flex-wrap items-end gap-8">
            <div>
              <p className="mb-2 text-xs font-medium text-[var(--brand-muted-on-subtle)]">
                Infraestructura tecnológica
              </p>
              <Image
                src="/brands/powered-by-aws.png"
                alt="Powered by AWS"
                width={108}
                height={38}
                className="h-8 w-auto object-contain"
              />
            </div>
            <div>
              <p className="mb-2 text-xs font-medium text-[var(--brand-muted-on-subtle)]">
                Pagos procesados por
              </p>
              <Image
                src="/brands/mercado-pago.png"
                alt="Mercado Pago"
                width={120}
                height={32}
                className="h-8 w-auto object-contain"
              />
            </div>
          </div>
          <p className="max-w-xl text-xs leading-relaxed text-[var(--brand-muted-on-subtle)]">
            La tarifa y modalidad dependen de cada publicación. Una estimación
            no confirma disponibilidad ni constituye cotización o reserva.
          </p>
        </div>

        <div className="border-t border-border/70 py-4 text-xs text-[var(--brand-muted-on-subtle)]">
          © {year} EspaciGo. Todos los derechos reservados.
        </div>
      </div>
    </footer>
  );
}
