import { Routes, Route } from "react-router-dom";
import { AppShell } from "@/components/app-shell";
import { OverviewPage } from "@/pages/overview";
import { PlaceholderPage } from "@/pages/placeholder";

export default function App() {
  return (
    <Routes>
      <Route element={<AppShell />}>
        <Route index element={<OverviewPage />} />
        <Route
          path="search"
          element={
            <PlaceholderPage
              title="Search"
              blurb="Query, filter and explore logs across streams and time ranges."
            />
          }
        />
        <Route
          path="streams"
          element={
            <PlaceholderPage
              title="Streams"
              blurb="Route messages into named categories with match rules and retention."
            />
          }
        />
        <Route
          path="dashboards"
          element={
            <PlaceholderPage
              title="Dashboards"
              blurb="Compose widgets into shareable, auto-refreshing dashboards."
            />
          }
        />
        <Route
          path="alerts"
          element={
            <PlaceholderPage
              title="Alerts"
              blurb="Define events and notify Shoutrrr, WhatsApp, webhook and email channels."
            />
          }
        />
        <Route
          path="inputs"
          element={
            <PlaceholderPage
              title="Inputs"
              blurb="Receive logs over Syslog, GELF, Beats, CEF, HTTP-JSON, NetFlow and IPFIX."
            />
          }
        />
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
