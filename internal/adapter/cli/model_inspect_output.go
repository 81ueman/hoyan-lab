package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/81ueman/hoyan-lab/internal/domain/model"
	"github.com/81ueman/hoyan-lab/internal/usecase/modelinspect"
)

func writePrefixClassTable(out io.Writer, rows []modelinspect.PrefixClassRow, showPreds bool) error {
	tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	if showPreds {
		fmt.Fprintln(tw, "CLASS\tSPACE\tMATCHED-PREDICATES")
	} else {
		fmt.Fprintln(tw, "CLASS\tSPACE")
	}
	for _, row := range rows {
		fmt.Fprintf(tw, "pc-%d\t%s",
			row.ClassID,
			row.Space,
		)
		if showPreds {
			fmt.Fprintf(tw, "\t%s", strings.Join(row.MatchedPredicates, ","))
		}
		fmt.Fprintln(tw)
	}
	return tw.Flush()
}

func writePacketClassTable(out io.Writer, rows []modelinspect.PacketClassRow, showPreds bool) error {
	tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	if showPreds {
		fmt.Fprintln(tw, "CLASS\tPREFIX-CLASS\tSPACE\tPROTOCOL\tSRC-PORT\tDST-PORT\tINGRESS\tEGRESS\tMATCHED-PREDICATES")
	} else {
		fmt.Fprintln(tw, "CLASS\tPREFIX-CLASS\tSPACE\tPROTOCOL\tSRC-PORT\tDST-PORT\tINGRESS\tEGRESS")
	}
	for _, row := range rows {
		fmt.Fprintf(tw, "pkt-%d\tpc-%d\t%s\t%s\t%s\t%s\t%s\t%s",
			row.ClassID,
			row.PrefixClassID,
			row.Space,
			row.Protocol,
			row.SrcPort,
			row.DstPort,
			row.IngressInterface,
			row.EgressInterface,
		)
		if showPreds {
			fmt.Fprintf(tw, "\t%s", strings.Join(row.MatchedPredicates, ","))
		}
		fmt.Fprintln(tw)
	}
	return tw.Flush()
}

func writeRIBTable(out io.Writer, rows []modelinspect.RIBRow, showCond bool, protocol model.RouteSourceKind) error {
	if protocol != "" && protocol != model.RouteSourceBGP {
		return writeRouteSourceRIBTable(out, rows, showCond)
	}
	tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	if showCond {
		fmt.Fprintln(tw, "NODE\tPREFIX\tSOURCE\tCLASS\tNEXT-HOP\tIFACE\tORIGIN\tFROM\tAS-PATH\tLOCAL-PREF\tMED\tORIGIN-CODE\tIBGP\tINVALID\tPATH\tCONDITION\tSELECTED")
	} else {
		fmt.Fprintln(tw, "NODE\tPREFIX\tSOURCE\tCLASS\tNEXT-HOP\tIFACE\tORIGIN\tFROM\tAS-PATH\tLOCAL-PREF\tMED\tORIGIN-CODE\tIBGP\tINVALID\tPATH")
	}
	for _, row := range rows {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s",
			row.Node,
			row.Prefix,
			row.SourceKind,
			row.ConnectedClass,
			row.NextHopNode,
			row.RouteInterface,
			row.OriginNode,
			row.FromNode,
			formatASPath(row.ASPath),
			formatIntPtr(row.LocalPref),
			formatIntPtr(row.MED),
			formatStringPtr(row.OriginCode),
			formatBoolPtr(row.LearnedIBGP),
			formatBoolPtr(row.Invalid),
			strings.Join(row.PathNodes, "->"),
		)
		if showCond {
			fmt.Fprintf(tw, "\t%s\t%s", row.Condition, row.SelectedCondition)
		}
		fmt.Fprintln(tw)
	}
	return tw.Flush()
}

func writeRouteSourceRIBTable(out io.Writer, rows []modelinspect.RIBRow, showCond bool) error {
	tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	if showCond {
		fmt.Fprintln(tw, "NODE\tPREFIX\tSOURCE\tCLASS\tOSPF-TYPE\tMETRIC\tNEXT-HOP\tIFACE\tORIGIN\tFROM\tPATH\tCONDITION\tSELECTED")
	} else {
		fmt.Fprintln(tw, "NODE\tPREFIX\tSOURCE\tCLASS\tOSPF-TYPE\tMETRIC\tNEXT-HOP\tIFACE\tORIGIN\tFROM\tPATH")
	}
	for _, row := range rows {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s",
			row.Node,
			row.Prefix,
			row.SourceKind,
			row.ConnectedClass,
			row.OSPFRouteType,
			formatIntPtr(row.Metric),
			row.NextHopNode,
			row.RouteInterface,
			row.OriginNode,
			row.FromNode,
			strings.Join(row.PathNodes, "->"),
		)
		if showCond {
			fmt.Fprintf(tw, "\t%s\t%s", row.Condition, row.SelectedCondition)
		}
		fmt.Fprintln(tw)
	}
	return tw.Flush()
}

func formatStringPtr(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

func formatIntPtr(v *int) string {
	if v == nil {
		return ""
	}
	return fmt.Sprintf("%d", *v)
}

func formatBoolPtr(v *bool) string {
	if v == nil {
		return ""
	}
	return fmt.Sprintf("%t", *v)
}

func writeFIBTable(out io.Writer, rows []modelinspect.FIBRow, showCond bool) error {
	tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	if showCond {
		fmt.Fprintln(tw, "NODE\tPREFIX\tSOURCE\tDISCARD\tCLASS\tNEXT-HOP\tRAW-NH\tNH-ADDR\tRESOLUTION\tIFACE\tRANK\tGROUP\tEQUIV\tCOST\tPATH\tLINKS\tCONDITION")
	} else {
		fmt.Fprintln(tw, "NODE\tPREFIX\tSOURCE\tDISCARD\tCLASS\tNEXT-HOP\tRAW-NH\tNH-ADDR\tRESOLUTION\tIFACE\tRANK\tGROUP\tEQUIV\tCOST\tPATH\tLINKS")
	}
	for _, row := range rows {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%t\t%s\t%s\t%s\t%s\t%s\t%s\t%d\t%s\t%t\t%d\t%s\t%s",
			row.Node,
			row.Prefix,
			row.SourceKind,
			row.Discard,
			row.ConnectedClass,
			row.NextHop,
			row.RawNextHop,
			row.NextHopAddress,
			row.ResolutionStatus,
			row.Interface,
			row.Rank,
			row.GroupID,
			row.Equivalent,
			row.Cost,
			strings.Join(row.PathNodes, "->"),
			strings.Join(row.PathLinks, "->"),
		)
		if showCond {
			fmt.Fprintf(tw, "\t%s", row.Condition)
		}
		fmt.Fprintln(tw)
	}
	return tw.Flush()
}

func writeSymbolicPacketTable(out io.Writer, result modelinspect.SymbolicPacketInspect, showCond bool) error {
	fmt.Fprintf(out, "from: %s\n", result.From)
	fmt.Fprintf(out, "to: %s\n", result.To)
	fmt.Fprintf(out, "protocol: %s\n", result.Protocol)
	if showCond {
		fmt.Fprintf(out, "reachable: %s\n", result.Reachable)
		fmt.Fprintf(out, "unreachable: %s\n", result.Unreachable)
	}
	if result.Reason != "" {
		fmt.Fprintf(out, "reason: %s\n", result.Reason)
	}
	if len(result.UnreachableReasons) > 0 {
		fmt.Fprintln(out, "blocked/unreachable reasons:")
		rtw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
		if showCond {
			fmt.Fprintln(rtw, "KIND\tNODE\tLINK\tINTERFACE\tPOLICY\tCONDITION\tPATH\tMESSAGE")
		} else {
			fmt.Fprintln(rtw, "KIND\tNODE\tLINK\tINTERFACE\tPOLICY\tPATH\tMESSAGE")
		}
		for _, reason := range result.UnreachableReasons {
			fmt.Fprintf(rtw, "%s\t%s\t%s\t%s\t%s",
				reason.Kind,
				reason.Node,
				reason.Link,
				reason.Interface,
				reason.PolicyName,
			)
			if showCond {
				fmt.Fprintf(rtw, "\t%s", reason.Condition)
			}
			fmt.Fprintf(rtw, "\t%s\t%s\n",
				strings.Join(reason.PathNodes, "->"),
				reason.Message,
			)
		}
		if err := rtw.Flush(); err != nil {
			return err
		}
	}
	tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	if showCond {
		fmt.Fprintln(tw, "PATH\tCOST\tCONDITION\tHOPS")
	} else {
		fmt.Fprintln(tw, "PATH\tCOST\tHOPS")
	}
	for _, path := range result.Paths {
		var hops []string
		for _, state := range path.States {
			hop := state.Node
			if state.IngressInterface != "" {
				hop += "(" + state.IngressInterface + ")"
			}
			hops = append(hops, hop)
		}
		fmt.Fprintf(tw, "%s\t%d",
			strings.Join(path.PathNodes, "->"),
			path.Cost,
		)
		if showCond {
			fmt.Fprintf(tw, "\t%s", path.Condition)
		}
		fmt.Fprintf(tw, "\t%s\n", strings.Join(hops, "->"))
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	if len(result.Blocked) == 0 {
		return nil
	}
	fmt.Fprintln(out, "blocked:")
	blockedTW := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	if showCond {
		fmt.Fprintln(blockedTW, "PATH\tCOST\tCONDITION\tACL\tNODE\tINTERFACE\tSTAGE\tSOURCE\tREASON")
	} else {
		fmt.Fprintln(blockedTW, "PATH\tCOST\tACL\tNODE\tINTERFACE\tSTAGE\tSOURCE\tREASON")
	}
	for _, path := range result.Blocked {
		fmt.Fprintf(blockedTW, "%s\t%d",
			strings.Join(path.PathNodes, "->"),
			path.Cost,
		)
		if showCond {
			fmt.Fprintf(blockedTW, "\t%s", path.Condition)
		}
		fmt.Fprintf(blockedTW, "\t%s\t%s\t%s\t%s\t%s\t%s\n",
			path.ACL,
			path.Node,
			path.Interface,
			path.Stage,
			formatConfigSource(path.Source),
			path.Reason,
		)
	}
	return blockedTW.Flush()
}

func formatConfigSource(src model.ConfigSource) string {
	var parts []string
	if src.Vendor != "" {
		parts = append(parts, src.Vendor)
	}
	if src.File != "" {
		file := src.File
		if src.Line > 0 {
			file = fmt.Sprintf("%s:%d", file, src.Line)
		}
		parts = append(parts, file)
	}
	if src.Raw != "" {
		parts = append(parts, src.Raw)
	}
	return strings.Join(parts, " ")
}

func writeSymbolicRouteTable(out io.Writer, results []modelinspect.SymbolicRouteInspect, showCond bool, showPreds bool) error {
	for i, result := range results {
		if i > 0 {
			fmt.Fprintln(out)
		}
		fmt.Fprintf(out, "from: %s\n", result.From)
		fmt.Fprintf(out, "prefix: %s\n", result.Prefix)
		fmt.Fprintf(out, "class: pc-%d\n", result.ClassID)
		fmt.Fprintf(out, "space: %s\n", result.Space)
		if showPreds && len(result.MatchedPredicates) > 0 {
			fmt.Fprintf(out, "matched predicates: %s\n", strings.Join(result.MatchedPredicates, ", "))
		}
		if showCond {
			fmt.Fprintf(out, "reachable: %s\n", result.Reachable)
			fmt.Fprintf(out, "unreachable: %s\n", result.Unreachable)
		}
		if result.Reason != "" {
			fmt.Fprintf(out, "reason: %s\n", result.Reason)
		}
		tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
		if showCond {
			fmt.Fprintln(tw, "PATH\tCOST\tLINKS\tCONDITION")
		} else {
			fmt.Fprintln(tw, "PATH\tCOST\tLINKS")
		}
		for _, path := range result.Paths {
			fmt.Fprintf(tw, "%s\t%d\t%s",
				strings.Join(path.PathNodes, "->"),
				path.Cost,
				strings.Join(path.PathLinks, "->"),
			)
			if showCond {
				fmt.Fprintf(tw, "\t%s", path.Condition)
			}
			fmt.Fprintln(tw)
		}
		if err := tw.Flush(); err != nil {
			return err
		}
	}
	return nil
}

func writeJSON(out io.Writer, value any) error {
	enc := json.NewEncoder(out)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	return enc.Encode(value)
}

func formatASPath(path []uint32) string {
	if len(path) == 0 {
		return ""
	}
	parts := make([]string, 0, len(path))
	for _, asn := range path {
		parts = append(parts, fmt.Sprint(asn))
	}
	return strings.Join(parts, " ")
}
