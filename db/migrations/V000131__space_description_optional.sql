-- Empty descriptions preserve the existing non-null text representation.
ALTER TABLE public.espacio DROP CONSTRAINT espacio_descripcion_check;
