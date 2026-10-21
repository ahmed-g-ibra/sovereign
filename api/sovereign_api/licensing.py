"""Licence metadata attached to every published measurement.

A licence that lives only on a web page gets separated from the data the
moment someone copies a JSON response into a notebook. Attaching it to the
response means the terms travel with the numbers.
"""

from __future__ import annotations

REPO = "https://github.com/ahmed-g-ibra/sovereign"

DATA_LICENSE = {
    "name": "CC BY 4.0",
    "url": "https://creativecommons.org/licenses/by/4.0/",
    "attribution": f"Sovereign — {REPO}",
    "note": ("Measurement data is CC BY 4.0. The software is MIT and licensed "
             "separately. Cite the date the data was taken: latency ages."),
}
