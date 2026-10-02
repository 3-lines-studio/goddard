import { ChartColumnIcon } from "lucide-react";

import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Empty, EmptyDescription, EmptyHeader, EmptyMedia, EmptyTitle } from "@/components/ui/empty";
import type { Day, Summary, Totals } from "./types";

const number = new Intl.NumberFormat("es-AR");

function duration(ms: number): string {
  if (ms < 1000) return `${number.format(ms)} ms`;
  const seconds = Math.round(ms / 1000);
  if (seconds < 60) return `${number.format(seconds)} s`;
  return `${number.format(Math.floor(seconds / 60))} m ${number.format(seconds % 60)} s`;
}

function Line({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex items-baseline justify-between gap-4">
      <span className="text-xs text-muted-foreground">{label}</span>
      <span className="text-sm font-medium tabular-nums">{value}</span>
    </div>
  );
}

function Numbers({ totals }: { totals: Totals }) {
  return (
    <div className="flex flex-col gap-1">
      <Line label="turnos" value={number.format(totals.turns)} />
      <Line label="tokens de entrada" value={number.format(totals.input)} />
      <Line label="tokens de salida" value={number.format(totals.output)} />
      <Line label="de entrada en caché" value={number.format(totals.cached_input)} />
      <Line label="tiempo" value={duration(totals.ms)} />
      <Line label="fallas" value={number.format(totals.failed)} />
      <Line label="cortados" value={number.format(totals.cancelled)} />
    </div>
  );
}

function DayCard({ day }: { day: Day }) {
  return (
    <Card size="sm" data-day={day.date}>
      <CardHeader>
        <CardTitle>{day.date}</CardTitle>
        <CardDescription>{day.model}</CardDescription>
        {day.failed > 0 ? <Badge variant="destructive">{day.failed} con error</Badge> : null}
      </CardHeader>
      <CardContent>
        <Numbers totals={day} />
      </CardContent>
    </Card>
  );
}

export function Telemetry({ summary }: { summary: Summary | null }) {
  if (!summary) {
    return (
      <Empty className="mx-auto max-w-3xl border">
        <EmptyHeader>
          <EmptyMedia variant="icon">
            <ChartColumnIcon />
          </EmptyMedia>
          <EmptyTitle>Leyendo los números</EmptyTitle>
          <EmptyDescription>Los turnos quedan en la base; esto los suma.</EmptyDescription>
        </EmptyHeader>
      </Empty>
    );
  }
  if (summary.days.length === 0) {
    return (
      <Empty className="mx-auto max-w-3xl border">
        <EmptyHeader>
          <EmptyMedia variant="icon">
            <ChartColumnIcon />
          </EmptyMedia>
          <EmptyTitle>Todavía no hay turnos en esta ventana</EmptyTitle>
          <EmptyDescription>Los que corran de acá en adelante van a aparecer acá.</EmptyDescription>
        </EmptyHeader>
      </Empty>
    );
  }
  const days = Math.round((summary.until - summary.from) / 86400);
  return (
    <div className="mx-auto flex w-full max-w-3xl flex-col gap-3 p-4">
      <section className="flex flex-col gap-1">
        <h2 className="text-sm font-semibold">Telemetría</h2>
        <p className="text-sm text-muted-foreground">
          Los turnos de todos los dueños, en números. Sin contenido y sin nombres.
        </p>
      </section>
      <Card size="sm" data-telemetry-total="">
        <CardHeader>
          <CardTitle>Los últimos {days} días</CardTitle>
          <CardDescription>{number.format(summary.days.length)} renglones de día y modelo</CardDescription>
        </CardHeader>
        <CardContent>
          <Numbers totals={summary.total} />
        </CardContent>
      </Card>
      {summary.days.map((day) => (
        <DayCard key={`${day.date}-${day.model}`} day={day} />
      ))}
    </div>
  );
}
