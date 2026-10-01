export default function Page({ title }: { title: string }) {
  return (
    <>
      <h1 className="text-2xl font-semibold">{title}</h1>
      <p className="mt-2">Todavía no hay nada acá. El agente llega después.</p>
    </>
  );
}
