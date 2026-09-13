package infinitescroll

import (
	"github.com/CoreyCole/vamos/pkg/datastarui/utils"
)

func InfiniteScrollVariants(args InfiniteScrollArgs) string {
	baseClasses := "relative"
	if args.Class != "" {
		return utils.TwMerge(baseClasses, args.Class)
	}
	return baseClasses
}

func HostVariants(args HostArgs) string {
	baseClasses := "relative"
	if args.Class != "" {
		return utils.TwMerge(baseClasses, args.Class)
	}
	return baseClasses
}

func ItemsVariants(args ItemsArgs) string {
	baseClasses := "space-y-0"
	if args.Class != "" {
		return utils.TwMerge(baseClasses, args.Class)
	}
	return baseClasses
}

func SentinelVariants(args SentinelArgs) string {
	baseClasses := "h-px w-full"
	if args.Class != "" {
		return utils.TwMerge(baseClasses, args.Class)
	}
	return baseClasses
}

func LoadingVariants(args LoadingArgs) string {
	baseClasses := "flex items-center justify-center p-4"
	if args.Class != "" {
		return utils.TwMerge(baseClasses, args.Class)
	}
	return baseClasses
}
