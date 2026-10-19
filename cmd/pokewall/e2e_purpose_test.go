package main

import (
 "encoding/json"
 "path/filepath"
 "testing"

 "github.com/maestroi/pokepilot/farm"
)

func TestE2EPurposeAcceptedAndStoredInHistoryCatalog(t *testing.T) {
 spec := farm.Spec{RunID:"e2e-db-check", Game:"pokemon-red", Planner:"llm", Purpose:farm.RunPurposeFullE2E, Goal:farm.GoalFrom("Beat the Elite Four and Champion.")}
 if !spec.Purpose.Valid() { t.Fatal("full_e2e purpose rejected") }
 raw, err := json.Marshal(spec)
 if err != nil { t.Fatal(err) }
 var roundtrip farm.Spec
 if err := json.Unmarshal(raw, &roundtrip); err != nil { t.Fatal(err) }
 if roundtrip.Purpose != farm.RunPurposeFullE2E { t.Fatalf("lost E2E tag: %s",raw) }

 catalog, _, err := openRunCatalog(filepath.Join(t.TempDir(), "catalog.sqlite"))
 if err != nil {t.Fatal(err)}
 defer catalog.db.Close()
 for _, row := range []tileRow{
  {RunID:"e2e-1",Status:statusDone,Game:"pokemon-red",Planner:"llm",Purpose:farm.RunPurposeFullE2E,Goal:"Beat the Elite Four and Champion.",EndedAt:100},
  {RunID:"ordinary-1",Status:statusDone,Game:"pokemon-red",Planner:"llm",Purpose:farm.RunPurposeNormal,Goal:"Beat the Elite Four and Champion.",EndedAt:200},
  {RunID:"debug-1",Status:statusDone,Game:"pokemon-red",Planner:"llm",Purpose:farm.RunPurposeDebugCoverage,EndedAt:300},
 } {
  if err := catalog.upsert(row);err != nil {t.Fatal(err)}
 }
 recovered,ok,err:=catalog.get("e2e-1")
 if err!=nil || !ok || recovered.Purpose!=farm.RunPurposeFullE2E {t.Fatalf("catalog lookup: row=%+v ok=%v err=%v",recovered,ok,err)}

 w:=NewWall("")
 wallCatalogs.Store(w,catalog)
 defer wallCatalogs.Delete(w)
 view,err:=w.catalogHistorySnapshot(runtimeDashboardQuery{status:statusDone,purpose:"full_e2e",limit:5})
 if err!=nil {t.Fatal(err)}
 if view.Total!=1 ||len(view.Runs)!=1 || view.Runs[0].RunID!="e2e-1" {
  t.Fatalf("database-filtered E2E history=%+v, want only tagged run",view)
 }
}
