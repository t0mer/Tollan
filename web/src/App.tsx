import { Routes, Route } from "react-router-dom";
import { AppShell } from "@/components/app-shell";
import { OverviewPage } from "@/pages/overview";
import { SearchPage } from "@/pages/search";
import { InputsPage } from "@/pages/inputs";
import { StreamsPage } from "@/pages/streams";
import { PipelinesPage } from "@/pages/pipelines";
import { DashboardsPage } from "@/pages/dashboards";
import { DashboardViewPage } from "@/pages/dashboard-view";
import { PlaceholderPage } from "@/pages/placeholder";

export default function App() {
  return (
    <Routes>
      <Route element={<AppShell />}>
        <Route index element={<OverviewPage />} />
        <Route path="search" element={<SearchPage />} />
        <Route path="streams" element={<StreamsPage />} />
        <Route path="pipelines" element={<PipelinesPage />} />
        <Route path="dashboards" element={<DashboardsPage />} />
        <Route path="dashboards/:id" element={<DashboardViewPage />} />
        <Route
          path="alerts"
          element={
            <PlaceholderPage
              title="Alerts"
              blurb="Define events and notify Shoutrrr, WhatsApp, webhook and email channels."
            />
          }
        />
        <Route path="inputs" element={<InputsPage />} />
        <Route
          path="fleet"
          element={
            <PlaceholderPage
              title="Fleet"
              blurb="Manage tollan-agent collectors and their centralized configuration."
            />
          }
        />
        <Route
          path="system"
          element={
            <PlaceholderPage
              title="System"
              blurb="Users, roles, API tokens, lookup tables, content packs and settings."
            />
          }
        />
        <Route path="*" element={<PlaceholderPage title="Not found" blurb="Nothing here." />} />
      </Route>
    </Routes>
  );
}
