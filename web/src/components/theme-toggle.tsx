import { Monitor, Moon, Sun } from "lucide-react";
import { useTheme } from "@/components/theme-provider";
import { Button } from "@/components/ui/button";

/** ThemeToggle cycles system → light → dark, matching the AGH/house standard of
 * a system-aware toggle with a manual override. */
export function ThemeToggle() {
  const { theme, setTheme } = useTheme();
  const next = theme === "system" ? "light" : theme === "light" ? "dark" : "system";
  const label = `Theme: ${theme}. Switch to ${next}.`;
  const Icon = theme === "system" ? Monitor : theme === "light" ? Sun : Moon;
  return (
    <Button
      variant="ghost"
      size="icon"
      aria-label={label}
      title={label}
      onClick={() => setTheme(next)}
    >
      <Icon />
    </Button>
  );
}
