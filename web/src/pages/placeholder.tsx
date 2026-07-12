import { Card, CardContent, CardHeader, CardTitle, CardDescription } from "@/components/ui/card";

/** PlaceholderPage renders a titled shell for a feature area that a later build
 * phase fills in. */
export function PlaceholderPage({ title, blurb }: { title: string; blurb: string }) {
  return (
    <div className="mx-auto max-w-5xl space-y-6">
      <div>
        <h1 className="font-display text-2xl font-semibold tracking-tight">{title}</h1>
        <p className="mt-1 text-sm text-muted-foreground">{blurb}</p>
      </div>
      <Card>
        <CardHeader>
          <CardTitle>Coming soon</CardTitle>
          <CardDescription>
            This area is scaffolded and will be wired up in an upcoming phase.
          </CardDescription>
        </CardHeader>
        <CardContent className="font-mono text-sm text-muted-foreground">
          {title.toLowerCase()} · not yet implemented
        </CardContent>
      </Card>
    </div>
  );
}
