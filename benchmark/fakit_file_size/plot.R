#!/usr/bin/env Rscript
library(argparse)
library(ggplot2)
library(dplyr)
library(scales)
library(ggthemes)
library(ggrepel)

parser <-
  ArgumentParser(description = "", formatter_class = "argparse.RawTextHelpFormatter")
parser$add_argument("-i", "--infile", type = "character",
                    help = "result file generated by run.pl")
parser$add_argument("-o", "--outfile", type = "character",
                    default = "",
                    help = "result figure file")
parser$add_argument("--width", type = "double",
                    default = 8,
                    help = "result file width")
parser$add_argument("--height", type = "double",
                    default = 6,
                    help = "result file height")
parser$add_argument("--dpi", type = "integer",
                    default = 300,
                    help = "DPI")

args <- parser$parse_args()

if (is.null(args$infile)) {
  write("ERROR: Input file (generated by run.pl) needed!\n", file = stderr())
  quit("no", 1)
}
if (args$outfile == "") {
  args$outfile = paste(args$infile, ".png", sep="")
}

w <- args$width
h <- args$height

df <- read.csv(args$infile, sep = "\t")

# sort
df$test <- factor(df$test, levels = unique(df$test), ordered = TRUE)
df$app <- factor(df$app, levels = unique(df$app), ordered = TRUE)
df$dataset <-
  factor(df$dataset, levels = unique(df$dataset), ordered = TRUE)

# rename dataset
re <- function(x) {
  if (is.character(x) | is.factor(x)) {
    x <- gsub("\\.fa","",x)
  }
  return(x)
}
df <- as.data.frame(lapply(df, re))


# humanize mem unit
max_mem <- max(df$mem_mean)
unit <- "KB"
if (max_mem > 1024 * 1024) {
  df <- df %>% mutate(mem_mean2 = mem_mean / 1024 / 1024)
  unit <- "GB"
} else if (max_mem > 1024) {
  df <- df %>% mutate(mem_mean2 = mem_mean / 1024)
  unit <- "MB"
} else {
  df <- df %>% mutate(mem_mean2 = mem_mean / 1)
  unit <- "KB"
}

p <-
  ggplot(df, aes(
    x = mem_mean2, y = time_mean,
    label = app
  )) +
  
  geom_point(size = 2) +
  geom_hline(aes(yintercept = time_mean), size = 0.1, alpha = 0.4) +
  geom_vline(aes(xintercept = mem_mean2), size = 0.1, alpha = 0.4) +
  geom_text_repel(size = 4) +
  scale_color_wsj() +
  facet_wrap( ~ test) +
  ylim(0, max(df$time_mean)) +
  xlim(0, max(df$mem_mean2)) +
  
  # ggtitle("Performance on different size of files") +
  ylab("Time (s)") +
  xlab(paste("Peak Memory (", unit, ")", sep = ""))

p <- p +
  theme_bw() +
  theme(
    panel.border = element_rect(color = "black", size = 1.2),
    panel.background = element_blank(),
    panel.grid.major = element_blank(),
    panel.grid.minor = element_blank(),
    axis.ticks.y = element_line(size = 0.8),
    axis.ticks.x = element_line(size = 0.8),
    
    strip.background = element_rect(
      colour = "white", fill = "white",
      size = 0.2
    ),
    
    legend.text = element_text(size = 14),
    legend.position = "c(0.85,0.25)",
    legend.background = element_rect(fill = "transparent"),
    legend.key.size = unit(0.6, "cm"),
    legend.key = element_blank(),
    legend.title = element_blank(),
    legend.text.align = 0,
    legend.box.just = "left",
    strip.text.x = element_text(angle = 0, hjust = 0),
    
    text = element_text(
      size = 14, family = "arial", face = "bold"
    ),
    plot.title = element_text(size = 15)
  ) 

ggsave(p, file = args$outfile, width = w, height = h, dpi=args$dpi)

#   p <- p + scale_color_manual(values = rep("black", length(df$app)))
#
#   ggsave(
#     p, file = paste("benchmark-", gsub(" ", "-", tolower(test1)), ".grey.png", sep = ""), width = w, height = h
#   )
